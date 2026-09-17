module github.com/uchaloop/confmaker

go 1.27

require github.com/uchaloop/secret/v2 v2.0.0

// v0.6.0 was tagged on the v0.5.0 commit by mistake and holds that code; the
// release it was meant to be is v0.6.1.
retract v0.6.0
