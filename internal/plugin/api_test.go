package plugin_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	pb "github.com/davidsbond/plugin/internal/generated/proto/plugin"
	"github.com/davidsbond/plugin/internal/plugin"
)

func TestAPI_Stat(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name     string
		Expected plugin.Info
		Request  *pb.StatRequest
	}{
		{
			Name: "returns plugin info",
			Expected: plugin.Info{
				Name:    "test-plugin",
				Version: "v0.1.0",
				Commands: []string{
					"a",
					"b",
					"c",
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			response, err := plugin.NewAPI(tc.Expected, nil).Stat(t.Context(), tc.Request)
			require.NoError(t, err)
			assert.EqualValues(t, tc.Expected.Name, response.GetName())
			assert.Equal(t, tc.Expected.Version, response.GetVersion())
			assert.EqualValues(t, tc.Expected.Commands, response.GetCommands())
		})
	}
}

func TestAPI_Execute(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name         string
		Request      *pb.ExecuteRequest
		Expected     *pb.ExecuteResponse
		ExpectsError bool
		ExpectedCode codes.Code
		Handlers     plugin.CommandHandlers
	}{
		{
			Name:         "command does not exist",
			ExpectsError: true,
			ExpectedCode: codes.NotFound,
			Handlers:     plugin.CommandHandlers{},
			Request: &pb.ExecuteRequest{
				Name: "test",
			},
		},
		{
			Name:         "empty command name",
			ExpectsError: true,
			ExpectedCode: codes.InvalidArgument,
			Handlers:     plugin.CommandHandlers{},
			Request: &pb.ExecuteRequest{
				Name: "",
			},
		},
		{
			Name:         "command returns error",
			ExpectsError: true,
			ExpectedCode: codes.Internal,
			Handlers: plugin.CommandHandlers{
				"test": func(ctx context.Context, input *anypb.Any) (*anypb.Any, error) {
					return nil, io.EOF
				},
			},
			Request: &pb.ExecuteRequest{
				Name: "test",
			},
		},
		{
			Name: "command succeeds",
			Handlers: plugin.CommandHandlers{
				"test": func(ctx context.Context, input *anypb.Any) (*anypb.Any, error) {
					return anypb.New(durationpb.New(time.Second))
				},
			},
			Request: &pb.ExecuteRequest{
				Name: "test",
			},
			Expected: &pb.ExecuteResponse{
				Output: mustAny(t, durationpb.New(time.Second)),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			response, err := plugin.NewAPI(plugin.Info{}, tc.Handlers).Execute(t.Context(), tc.Request)
			if tc.ExpectsError {
				require.Error(t, err)
				assert.EqualValues(t, tc.ExpectedCode, status.Code(err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.Expected, response)
		})
	}
}

func mustAny(t *testing.T, in proto.Message) *anypb.Any {
	t.Helper()

	out, err := anypb.New(in)
	require.NoError(t, err)
	return out
}
