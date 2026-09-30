module github.com/hasangenc0/bordo/control-plane

go 1.22

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/hasangenc0/bordo/build v0.0.0
	github.com/hasangenc0/bordo/release v0.0.0
	github.com/mattn/go-sqlite3 v1.14.52
	github.com/spf13/cobra v1.8.1
	golang.org/x/crypto v0.27.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/hasangenc0/bordo/build => ../build

replace github.com/hasangenc0/bordo/release => ../release

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.25.0 // indirect
)
