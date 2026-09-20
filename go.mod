module github.com/jcrussell/livermore-budget

go 1.25.0

// The language floor above is 1.25.0; this pins the toolchain that actually
// builds it. They are separate on purpose: govulncheck reports standard-library
// CVEs against whichever toolchain is installed, so a job held at exactly the
// floor stays red on every stdlib fix released since. Pinning here means the
// binary CI scans is the binary CI builds -- 1.25.8 is not enough (4 called
// stdlib vulnerabilities remain); 1.26.6 is clean. Bump this when govulncheck
// reports a finding, per byob-security.2.
toolchain go1.26.6

require (
	github.com/google/go-cmp v0.7.0
	github.com/google/jsonschema-go v0.4.3
	github.com/spf13/cobra v1.8.1
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
