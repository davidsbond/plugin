//go:generate go tool buf format -w .
//go:generate go tool buf generate

// Package plugin provides a gRPC-based plugin system allowing programs to dynamically execute custom code that satisfy
// the plugin interface.
//
// Plugins are external binaries that serve gRPC requests over a UNIX domain socket on the local machine. Each plugin
// creates a socket using a unique identifier passed as an argument from the host application to the plugin. Plugins
// make use of the protobuf "Any" type in order to allow user-defined inputs and outputs to keep strong typing across
// application and language boundaries.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/rs/xid"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/davidsbond/plugin/internal/plugin"
)

type (
	// The Config type contains fields used to configure a plugin.
	Config struct {
		// The Name of the plugin. This will be used to determine the location of the UNIX domain socket the plugin
		// will use for communication. It is typically created at /tmp/plugin_<name>.sock and is deleted when the
		// plugin exits.
		Name string
		// The Commands the plugin is capable of handling. When attempting to use a command that does not exist within
		// the plugin, an ErrUnknownCommand error is returned to the caller.
		Commands []CommandHandler
		// Any ServerOptions to apply to the gRPC server. This could be middleware, keepalives credentials etc.
		ServerOptions []grpc.ServerOption
	}

	// The CommandHandler interface describes types that act as individual commands a plugin can handle. Plugin authors should
	// not implement this interface directly and instead use the Command type.
	CommandHandler interface {
		// The Name of the Command.
		Name() string
		// Execute should perform any actions necessary to fulfil command execution.
		Execute(ctx context.Context, input *anypb.Any) (*anypb.Any, error)
	}

	// The Command type is a Command implementation that should be used by plugin authors to define their
	// individual commands. It uses parameterized types to avoid any type switching and ensure strong typing for
	// command implementations.
	Command[Input, Output proto.Message] struct {
		// Use describes the name of the command.
		Use string
		// Run is a function that is invoked when the plugin receives a request to execute the command.
		Run func(ctx context.Context, input Input) (Output, error)
	}
)

// Name returns the name of the command.
func (ch Command[Input, Output]) Name() string {
	return ch.Use
}

// Execute the command. This method handles all conversions from the protobuf Any type to those specified by the
// parameterized types provided by plugin authors.
func (ch Command[Input, Output]) Execute(ctx context.Context, input *anypb.Any) (*anypb.Any, error) {
	message, err := input.UnmarshalNew()
	if err != nil {
		return nil, err
	}

	in, ok := message.(Input)
	if !ok {
		return nil, fmt.Errorf("invalid input type for command %q", ch.Use)
	}

	output, err := ch.Run(ctx, in)
	if err != nil {
		return nil, err
	}

	out, err := anypb.New(output)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// Run a plugin using the provided configuration. This function blocks until the process receives an SIGINT, SIGTERM
// or SIGKILL signal. At which point it will gracefully stop the gRPC server and remove its UNIX domain socket.
func Run(config Config) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s [socket id]", config.Name),
		Version: getPluginVersion(),
		Short:   fmt.Sprintf("Starts the %q plugin", config.Name),
		Long:    fmt.Sprintf("Starts the %q plugin.\n\nOnce started, the plugin will begin listening for commands on a UNIX domain socket under /tmp. This socket name is specified by the first argument passed to the command.", config.Name),
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return startPlugin(cmd.Context(), config, args[0], cmd.Version)
		},
	}

	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Printf("failed to start plugin %q: %v\n", config.Name, err)
		os.Exit(1)
	}
}

func startPlugin(ctx context.Context, config Config, id, version string) error {
	server := grpc.NewServer(config.ServerOptions...)

	info := plugin.Info{
		Name:    config.Name,
		Version: version,
	}

	handlers := plugin.CommandHandlers{}
	for _, command := range config.Commands {
		handlers[command.Name()] = command.Execute
		info.Commands = append(info.Commands, command.Name())
	}

	plugin.NewAPI(info, handlers).Register(server)

	socket := "/tmp/" + id + ".sock"
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return server.Serve(listener)
	})

	group.Go(func() error {
		<-ctx.Done()
		server.GracefulStop()
		return listener.Close()
	})

	return group.Wait()
}

func getPluginVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}

	return "unknown"
}

type (
	// The Plugin type represents a running instance of a plugin referenced by the application that is invoking it. It
	// is intended to be used as a client for the plugin.
	Plugin struct {
		command *exec.Cmd
		err     error
		client  *plugin.Client
		info    plugin.Info
	}
)

var (
	// ErrUnexpectedName is the error given when a started plugin returns a name that is different to that of its
	// filename. For example, a plugin called "foo" whose binary is located at "/tmp/bar".
	ErrUnexpectedName = errors.New("unexpected plugin name")
)

// Use the plugin at the given path. This function executes the plugin binary which will begin serving gRPC requests
// on its UNIX domain socket. Once started, a small wait is performed to allow any startup actions the plugin requires
// before it is queried for its name, version and available commands.
//
// The name returned by the plugin must match the base of the given path. If they do not match, ErrUnexpectedName
// is returned.
//
// If successful, it is up to the caller to eventually call Plugin.Close when they no longer require use of the plugin.
func Use(ctx context.Context, path string) (*Plugin, error) {
	socket := xid.New().String()

	cmd := &exec.Cmd{
		Path: path,
		Args: []string{
			path,
			socket,
		},
	}

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start plugin at %q: %w", path, err)
	}

	p := &Plugin{
		command: cmd,
	}

	name := filepath.Base(path)
	p.client, err = plugin.NewClient(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to dial plugin %q: %w", name, err)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	<-ticker.C

	info, err := p.client.Stat(ctx)
	if err != nil {
		return nil, errors.Join(p.Close(), err)
	}

	if info.Name != name {
		return nil, fmt.Errorf("%w: expected %q, got %q", ErrUnexpectedName, name, info.Name)
	}

	p.info = info

	return p, nil
}

// Close the plugin. This method terminates the gRPC connection to the plugin and sends a SIGTERM signal to the process,
// allowing the plugin to gracefully shutdown.
func (p *Plugin) Close() error {
	var err error
	if p.client != nil {
		err = errors.Join(err, p.client.Close())
	}

	if p.command.Process != nil {
		err = errors.Join(err, p.command.Process.Signal(syscall.SIGTERM))
	}

	return err
}

var (
	// ErrUnknownCommand is an error returned by Plugin.Exec when attempting to execute a command that does not
	// exist within the plugin.
	ErrUnknownCommand = errors.New("unknown command")
)

// Exec executes the named command, providing a proto-encoded input. The provided input will be wrapped in a protobuf
// Any type. Returns ErrUnknownCommand if the specified command is unknown to the plugin. The command output will be
// unmarshalled directly into the provided output parameter.
func (p *Plugin) Exec(ctx context.Context, name string, input proto.Message, output proto.Message) error {
	err := p.client.Execute(ctx, name, input, output)
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %q", ErrUnknownCommand, name)
	}

	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			return errors.New(st.Message())
		}

		return err
	}

	return nil
}

// Commands returns all commands the Plugin provides.
func (p *Plugin) Commands() []string {
	return p.info.Commands
}

// Name returns the name of the Plugin.
func (p *Plugin) Name() string {
	return p.info.Name
}

// Version returns the version of the plugin.
func (p *Plugin) Version() string {
	return p.info.Version
}
