package plugin

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/davidsbond/plugin/internal/generated/proto/plugin"
)

type (
	// The Client type is used to communicate from the host application to the plugin via gRPC.
	Client struct {
		conn  *grpc.ClientConn
		inner plugin.PluginServiceClient
	}
)

// NewClient attempts to create a new connection to the plugin using the provided UNIX domain socket.
func NewClient(socket string) (*Client, error) {
	conn, err := grpc.NewClient("unix:///tmp/"+socket+".sock",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:  conn,
		inner: plugin.NewPluginServiceClient(conn),
	}, nil
}

// Close the connection to the plugin.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Stat returns plugin metadata, such as its name, version and commands it supports.
func (c *Client) Stat(ctx context.Context) (Info, error) {
	response, err := c.inner.Stat(ctx, &plugin.StatRequest{})
	if err != nil {
		return Info{}, err
	}

	return Info{
		Name:     response.GetName(),
		Version:  response.GetVersion(),
		Commands: response.GetCommands(),
	}, nil
}

// Execute a named command with the provided input. The command output will be unmarshalled into the provided output
// type.
func (c *Client) Execute(ctx context.Context, name string, input proto.Message, output proto.Message) error {
	i, err := anypb.New(input)
	if err != nil {
		return err
	}

	request := &plugin.ExecuteRequest{
		Name:  name,
		Input: i,
	}

	response, err := c.inner.Execute(ctx, request)
	if err != nil {
		return err
	}

	if err = response.GetOutput().UnmarshalTo(output); err != nil {
		return err
	}

	return nil
}
