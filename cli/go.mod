module github.com/bordo-io/bordo/cli

go 1.22

require (
	github.com/bordo-io/bordo/build v0.0.0
	github.com/spf13/cobra v1.8.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	nhooyr.io/websocket v1.8.17 // indirect
)

replace github.com/bordo-io/bordo/build => ../build
