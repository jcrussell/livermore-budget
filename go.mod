module github.com/jcrussell/livermore-budget

go 1.25.0

// The language floor above is 1.25.0; this pins the toolchain that builds it,
// so the binary CI scans with govulncheck is the binary CI builds. Bump it when
// govulncheck reports a standard-library finding, per byob-security.2.
toolchain go1.26.9

require (
	github.com/google/go-cmp v0.7.0
	github.com/google/jsonschema-go v0.4.3
	github.com/spf13/cobra v1.10.2
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/net v0.58.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)
