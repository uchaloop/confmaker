/*
Package confx provides configs loaded by github.com/uchaloop/confmaker to an Uber
Fx application.

	fx.New(
		confx.Module(),
		confx.Provide[postgres.Config](),
		confx.ProvideNamed[postgres.Config]("replica", confmaker.WithPrefix("REPLICA_POSTGRES_")),
	)

[Module] creates one confmaker.Loader per application and is required.
[Provide] registers a config and provides it untagged; [ProvideNamed] provides it
under the Fx tag name:"<name>". Every provided config is loaded when the
application starts, whether or not anything consumes it, and the start fails with
one error listing every problem.

Declaring configs, the options and the loading rules are documented in package
confmaker.
*/
package confx
