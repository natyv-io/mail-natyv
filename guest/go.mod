module natyv/mail-natyv-guest

go 1.23

require (
	github.com/extism/go-pdk v1.1.3
	github.com/natyv-io/sdks/go v0.0.0
)

// Manual, local-only override -- natyv-io/sdks is still private, so plain
// `go get` can't resolve it yet. Remove this once the repo goes public.
replace github.com/natyv-io/sdks/go => /Users/quinnmillican/Desktop/dev/natyv-io/sdks/go
