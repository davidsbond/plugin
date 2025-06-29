package main

import (
	"context"
	"log"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/davidsbond/plugin"
)

func main() {
	config := plugin.Config{
		Name: "test_plugin",
		Commands: []plugin.CommandHandler{
			&plugin.Command[*timestamppb.Timestamp, *timestamppb.Timestamp]{
				Use: "test",
				Run: func(ctx context.Context, input *timestamppb.Timestamp) (*timestamppb.Timestamp, error) {
					return input, nil
				},
			},
		},
	}

	if err := plugin.Run(config); err != nil {
		log.Fatal(err)
	}
}
