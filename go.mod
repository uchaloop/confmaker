module github.com/uchaloop/confmaker

go 1.27

require github.com/uchaloop/secret/v2 v2.0.0

// v0.6.0 and v0.6.1 were tagged on the v0.5.0 commit by mistake and hold that
// code; the release they were meant to be is v0.6.2.
retract [v0.6.0, v0.6.1]
