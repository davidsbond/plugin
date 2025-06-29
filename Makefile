test: test-plugin
	go test -race ./...

test-plugin:
	go build ./testdata/test_plugin
