/*
Package confcli provides an optional describe flag for configuration documentation.
Callers parse flags, check whether a description was requested, prepare a manifest
and pass it to the selected writer. This package does not load configuration,
evaluate defaults, manage files or exit the application.
*/
package confcli
