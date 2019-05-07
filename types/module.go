package types

import (
	"context"
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/cobra"
	"github.com/tendermint/go-crypto/keys"
	abci "github.com/tendermint/tendermint/abci/types"
)

//__________________________________________________________________________________________
// AppModule is the standard form for basic non-dependant elements of an application module
type AppModuleBasic interface {
	Name() string
	RegisterCodec(*codec.Codec)

	// genesis
	DefaultGenesis() json.RawMessage
	ValidateGenesis(json.RawMessage) error

	// client functionality
	RegisterRESTRoutes(context.CLIContext, *mux.Router, *codec.Codec, keys.Keybase)
	GetQueryCmd() *cobra.Command
	GetTxCmd() *cobra.Command
}

// collections of AppModuleBasic
type ModuleBasicManager []AppModuleBasic

func NewModuleBasicManager(modules ...AppModuleBasic) ModuleBasicManager {
	return modules
}

// RegisterCodecs registers all module codecs
func (mbm ModuleBasicManager) RegisterCodec(cdc *codec.Codec) {
	for _, mb := range mbm {
		mb.RegisterCodec(cdc)
	}
}

// Provided default genesis information for all modules
func (mbm ModuleBasicManager) DefaultGenesis() map[string]json.RawMessage {
	genesis := make(map[string]json.RawMessage)
	for _, mb := range mbm {
		genesis[mb.Name()] = mb.DefaultGenesis()
	}
	return genesis
}

// Provided default genesis information for all modules
func (mbm ModuleBasicManager) ValidateGenesis(genesis map[string]json.RawMessage) error {
	for _, mb := range mbm {
		if err := mb.ValidateGenesis(genesis[mb.Name()]); err != nil {
			return err
		}
	}
	return nil
}

// RegisterRestRoutes registers all module rest routes
func (mbm ModuleBasicManager) RegisterRESTRoutes(
	ctx context.CLIContext, rtr *mux.Router, cdc *codec.Codec, kb keys.Keybase) {

	for _, mb := range mbm {
		mb.RegisterRESTRoutes(ctx, rtr, cdc, kb)
	}
}

// add all tx commands to the rootTxCmd
func (mbm ModuleBasicManager) AddTxCommands(rootTxCmd *cobra.Command) {
	for _, mb := range mbm {
		if cmd := mb.GetTxCmd(); cmd != nil {
			rootTxCmd.AddCommand(cmd)
		}
	}
}

// add all query commands to the rootQueryCmd
func (mbm ModuleBasicManager) AddQueryCommands(rootQueryCmd *cobra.Command) {
	for _, mb := range mbm {
		if cmd := mb.GetQueryCmd(); cmd != nil {
			rootQueryCmd.AddCommand(cmd)
		}
	}
}

//_________________________________________________________
// AppModule is the standard form for an application module
type AppModule interface {
	AppModuleBasic

	// registers
	RegisterInvariants(InvariantRouter)

	// routes
	Route() string
	NewHandler() Handler
	QuerierRoute() string
	NewQuerierHandler() Querier

	// genesis
	InitGenesis(Context, json.RawMessage) []abci.ValidatorUpdate
	ExportGenesis(Context) json.RawMessage

	BeginBlock(Context, abci.RequestBeginBlock) Tags
	EndBlock(Context, abci.RequestEndBlock) ([]abci.ValidatorUpdate, Tags)
}

// module manager provides the high level utility for managing and executing
// operations for a group of modules
type ModuleManager struct {
	Modules            map[string]AppModule
	OrderInitGenesis   []string
	OrderExportGenesis []string
	OrderBeginBlockers []string
	OrderEndBlockers   []string
}

// NewModuleManager creates a new ModuleManager object
func NewModuleManager(modules ...AppModule) *ModuleManager {

	moduleMap := make(map[string]AppModule)
	var modulesStr []string
	for _, module := range modules {
		moduleMap[module.Name()] = module
		modulesStr = append(modulesStr, module.Name())
	}

	return &ModuleManager{
		Modules:            moduleMap,
		OrderInitGenesis:   modulesStr,
		OrderExportGenesis: modulesStr,
		OrderBeginBlockers: modulesStr,
		OrderEndBlockers:   modulesStr,
	}
}

// set the order of init genesis calls
func (mm *ModuleManager) SetOrderInitGenesis(moduleNames ...string) {
	mm.OrderInitGenesis = moduleNames
}

// set the order of export genesis calls
func (mm *ModuleManager) SetOrderExportGenesis(moduleNames ...string) {
	mm.OrderExportGenesis = moduleNames
}

// set the order of set begin-blocker calls
func (mm *ModuleManager) SetOrderBeginBlockers(moduleNames ...string) {
	mm.OrderBeginBlockers = moduleNames
}

// set the order of set end-blocker calls
func (mm *ModuleManager) SetOrderEndBlockers(moduleNames ...string) {
	mm.OrderEndBlockers = moduleNames
}

// register all module routes and module querier routes
func (mm *ModuleManager) RegisterInvariants(invarRouter InvariantRouter) {
	for _, module := range mm.Modules {
		module.RegisterInvariants(invarRouter)
	}
}

// register all module routes and module querier routes
func (mm *ModuleManager) RegisterRoutes(router Router, queryRouter QueryRouter) {
	for _, module := range mm.Modules {
		if module.Route() != "" {
			router.AddRoute(module.Route(), module.NewHandler())
		}
		if module.QuerierRoute() != "" {
			queryRouter.AddRoute(module.QuerierRoute(), module.NewQuerierHandler())
		}
	}
}

// perform init genesis functionality for modules
func (mm *ModuleManager) InitGenesis(ctx Context, genesisData map[string]json.RawMessage) abci.ResponseInitChain {
	var validatorUpdates []abci.ValidatorUpdate
	for _, moduleName := range mm.OrderInitGenesis {
		if genesisData[moduleName] == nil {
			continue
		}
		moduleValUpdates := mm.Modules[moduleName].InitGenesis(ctx, genesisData[moduleName])

		// use these validator updates if provided, the module manager assumes
		// only one module will update the validator set
		if len(moduleValUpdates) > 0 {
			validatorUpdates = moduleValUpdates
		}
	}
	return abci.ResponseInitChain{
		Validators: validatorUpdates,
	}
}

// perform export genesis functionality for modules
func (mm *ModuleManager) ExportGenesis(ctx Context) map[string]json.RawMessage {
	genesisData := make(map[string]json.RawMessage)
	for _, moduleName := range mm.OrderExportGenesis {
		genesisData[moduleName] = mm.Modules[moduleName].ExportGenesis(ctx)
	}
	return genesisData
}

// perform begin block functionality for modules
func (mm *ModuleManager) BeginBlock(ctx Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	tags := EmptyTags()
	for _, moduleName := range mm.OrderBeginBlockers {
		moduleTags := mm.Modules[moduleName].BeginBlock(ctx, req)
		tags = tags.AppendTags(moduleTags)
	}

	return abci.ResponseBeginBlock{
		Tags: tags.ToKVPairs(),
	}
}

// perform end block functionality for modules
func (mm *ModuleManager) EndBlock(ctx Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	validatorUpdates := []abci.ValidatorUpdate{}
	tags := EmptyTags()
	for _, moduleName := range mm.OrderEndBlockers {
		moduleValUpdates, moduleTags := mm.Modules[moduleName].EndBlock(ctx, req)
		tags = tags.AppendTags(moduleTags)

		// use these validator updates if provided, the module manager assumes
		// only one module will update the validator set
		if len(moduleValUpdates) > 0 {
			validatorUpdates = moduleValUpdates
		}
	}

	return abci.ResponseEndBlock{
		ValidatorUpdates: validatorUpdates,
		Tags:             tags,
	}
}

// DONTCOVER