module github.com/i2y/pyffi/examples/sessions

go 1.26

require github.com/i2y/pyffi/casdk v0.0.0

require (
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/i2y/pyffi v0.0.0 // indirect
)

replace (
	github.com/i2y/pyffi => ../../
	github.com/i2y/pyffi/casdk => ../../casdk
)
