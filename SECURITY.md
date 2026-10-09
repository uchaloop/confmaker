# Security policy

confmaker loads application configuration from ENV and provides declaration
manifests and load diagnostics. Applications supply the environment and
configuration types; confmaker does not fetch secrets or manage deployment
security.

## What to report privately

Use private reporting for suspected vulnerabilities, including:

- Configuration values exposed through library-generated errors or diagnostics
  contrary to documented guarantees.
- Sensitive defaults disclosed by manifest exports contrary to documented rules.
- Parsing or validation bypasses with a demonstrable security impact.
- Input that causes a denial of service across an application's trust boundary.

An ordinary validation error or a panic caused only by an invalid
application-defined type is not necessarily a vulnerability. Report ordinary
bugs through GitHub Issues. If the security impact is unclear, use private
reporting so it can be assessed before disclosure.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** action in the repository's Security tab:

[Report privately](https://github.com/uchaloop/confmaker/v2/security/advisories/new)

Do not publish exploit details in an issue or pull request before discussing
them privately with the maintainer.

Include:

- The confmaker and Go versions.
- A minimal reproduction using synthetic configuration values.
- Expected and actual behavior.
- The affected output or operation and its security impact.
- Which inputs an attacker can control, if applicable.

Never include real credentials, production ENV dumps or sensitive logs.

If private reporting is unavailable, open an issue asking the maintainer to
enable it, without vulnerability details.

## Scope and limitations

Non-secret defaults may appear in manifest exports by design. Applications
must mark sensitive fields using the documented mechanisms.

Application code, custom callbacks and external decoders may expose values
outside confmaker's guarantees. Reports should identify where disclosure or
incorrect handling occurs.

## Supported versions

Only the latest stable release receives security fixes. Older releases,
including previous major and minor versions, are unsupported. Fixes are not
backported; users of older versions must upgrade to the latest stable release.

The maintainer will assess reports and coordinate fixes and disclosure.
No response-time commitment is currently defined.
