module github.com/uchaloop/confmaker/integration/secret

go 1.27.0

require (
	github.com/uchaloop/confmaker/v2 v2.0.0
	github.com/uchaloop/secret/v2 v2.2.0
)

// Exercise the checked-out core, not a published copy.
replace github.com/uchaloop/confmaker/v2 => ../..
