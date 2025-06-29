package plugin

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/davidsbond/plugin/internal/generated/proto/plugin"
)

type (
	// The API type implements the plugin API, exposing plugin information and command execution.
	API struct {
		plugin.UnimplementedPluginServiceServer
		info     Info
		handlers CommandHandlers
	}

	// The CommandHandlers type is a map that stores command names against their execution functions.
	CommandHandlers map[string]func(ctx context.Context, input *anypb.Any) (*anypb.Any, error)

	// The Info type contains plugin-specific metadata.
	Info struct {
		// The Name of the plugin.
		Name string
		// The Version of the plugin.
		Version string
		// Commands provided by the plugin.
		Commands []string
	}
)

// NewAPI returns a new instance of the API type that will serve the provided plugin information and execute the
// provided command handlers.
func NewAPI(info Info, handlers CommandHandlers) *API {
	return &API{
		info:     info,
		handlers: handlers,
	}
}

// Register the API onto the grpc.ServiceRegistrar.
func (api *API) Register(s grpc.ServiceRegistrar) {
	plugin.RegisterPluginServiceServer(s, api)
}

// Stat returns metadata about the running plugin. Includes its name, version and the commands that can be executed.
func (api *API) Stat(context.Context, *plugin.StatRequest) (*plugin.StatResponse, error) {
	return &plugin.StatResponse{
		Name:     api.info.Name,
		Version:  api.info.Version,
		Commands: api.info.Commands,
	}, nil
}

// Execute the command describes within the request. Returns codes.NotFound if no command matching the given name
// is registered with the plugin.
func (api *API) Execute(ctx context.Context, request *plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	if request.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing command name")
	}
	
	handler, ok := api.handlers[request.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown command %q", request.GetName())
	}

	output, err := handler(ctx, request.GetInput())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &plugin.ExecuteResponse{Output: output}, nil
}
