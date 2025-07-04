package plugin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/davidsbond/plugin"
)

func TestUse(t *testing.T) {
	p, err := plugin.Use(t.Context(), "./test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		assert.NoError(t, p.Close())
	})

	assert.EqualValues(t, "test_plugin", p.Name())
	assert.NotEmpty(t, p.Version())

	if assert.Len(t, p.Commands(), 1) {
		assert.EqualValues(t, "pingpong", p.Commands()[0])
	}

	t.Run("command pings", func(t *testing.T) {
		input := wrapperspb.String("ping")
		output := &wrapperspb.StringValue{}
		err = p.Exec(t.Context(), "pingpong", input, output)

		require.NoError(t, err)
		assert.NotNil(t, output)
		assert.EqualValues(t, "pong", output.GetValue())
	})

	t.Run("command pongs", func(t *testing.T) {
		input := wrapperspb.String("pong")
		output := &wrapperspb.StringValue{}
		err = p.Exec(t.Context(), "pingpong", input, output)

		require.NoError(t, err)
		assert.NotNil(t, output)
		assert.EqualValues(t, "ping", output.GetValue())
	})

	t.Run("command errors if not ping or pong", func(t *testing.T) {
		input := wrapperspb.String("pung")
		output := &wrapperspb.StringValue{}
		err = p.Exec(t.Context(), "pingpong", input, output)

		assert.Error(t, err)
	})

	t.Run("unknown command", func(t *testing.T) {
		input := wrapperspb.String("pong")
		output := &wrapperspb.StringValue{}
		err = p.Exec(t.Context(), "unknown", input, output)

		require.Error(t, err)
		assert.True(t, errors.Is(err, plugin.ErrUnknownCommand))
	})

	t.Run("error if invalid input type", func(t *testing.T) {
		input := durationpb.New(time.Hour)
		output := &wrapperspb.StringValue{}

		err = p.Exec(t.Context(), "test", input, output)
		require.Error(t, err)
	})
}
