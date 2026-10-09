/*
Package confexport renders prepared confmaker manifests as JSON, Markdown or ENV
examples. Writers do not load configuration, evaluate defaults or invoke user
methods. Callers choose whether to include defaults when preparing the manifest.
Writer errors may leave partial output; callers own files and other resources.
*/
package confexport
