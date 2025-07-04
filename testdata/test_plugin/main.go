package main

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/davidsbond/plugin"
)

type (
	PingPongPlugin struct{}
)

func (tp *PingPongPlugin) Run() {
	plugin.Run(plugin.Config{
		Name: "test_plugin",
		Commands: []plugin.CommandHandler{
			&plugin.Command[*wrapperspb.StringValue, *wrapperspb.StringValue]{
				Use: "pingpong",
				Run: tp.PingPong,
			},
		},
	})
}

func (tp *PingPongPlugin) PingPong(ctx context.Context, input *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	if input.GetValue() == "ping" {
		return &wrapperspb.StringValue{Value: "pong"}, nil
	}

	if input.GetValue() == "pong" {
		return &wrapperspb.StringValue{Value: "ping"}, nil
	}

	return nil, fmt.Errorf(`invalid input %q, expected "ping" or "pong"`, input.Value)
}

func main() {
	(&PingPongPlugin{}).Run()
}
