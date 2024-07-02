
# atm (alertmanager tenants mute)

...aka Alertmanager silences distributor

This a dirty copy of [amtool](https://github.com/prometheus/alertmanager?tab=readme-ov-file#amtool) which allow to create silence for multi-tenants.

It support the same config file format, but has only the `silence add` cmd.

## usage

```
usage: atm silence add [<flags>] [<matcher-groups>...]

Add a new alertmanager silence

    atm uses a simplified Prometheus syntax to represent silences. The
    non-option section of arguments constructs a list of "Matcher Groups"
    that will be used to create a number of silences. The following examples
    will attempt to show this behaviour in action:

    atm silence add alertname=foo node=bar

    This statement will add a silence that matches alerts with the
    alertname=foo and node=bar label value pairs set.

    atm silence add foo node=bar

    If alertname is omitted and the first argument does not contain a '=' or a
    '=~' then it will be assumed to be the value of the alertname pair.

    atm silence add 'alertname=~foo.*'

    As well as direct equality, regex matching is also supported. The '=~' syntax
    (similar to Prometheus) is used to represent a regex match. Regex matching
    can be used in combination with a direct match.

Flags:
  -h, --[no-]help               Show context-sensitive help (also try --help-long and --help-man).
      --date.format="2006-01-02 15:04:05 MST"  
                                Format of date output
      --alertmanager.url=ALERTMANAGER.URL  
                                Alertmanager to talk to
      --timeout=30s             Timeout for the executed command
      --http.config.file=<filename>  
                                HTTP client configuration file for atm to connect to Alertmanager.
      --[no-]version            Show application version.
  -t, --tenant=TENANT           tenant
      --tenant.http-header="X-Scope-OrgID"
                                tenant HTTP Header
      --tenant.file=<filename>  tenant file location
  -a, --author="fgouteroux"     Username for CreatedBy field
  -d, --duration="1h"           Duration of silence
      --max-duration="12h"      Max Duration of silence
      --start=START             Set when the silence should start. RFC3339 format
                                2006-01-02T15:04:05-07:00
      --end=END                 Set when the silence should end (overwrites duration). RFC3339 format
                                2006-01-02T15:04:05-07:00
  -c, --comment=COMMENT         A comment to help describe the silence

Args:
  [<matcher-groups>]  Query filter

```

## How it works

When using grafana mimir with multi-tenancy enabled, we need to pass the HTTP tenant ID header.

atm take a tenants list file and iterate over each tenant, set the expected header ( `X-Scope-OrgID` by default) and create the silence.

We add a default max duration of 12h by default to avoid muting all tenants alerts for a long time.

### Create multi-tenants silences

examples/tenants.conf
```
tenant-a
tenant-b
```

Add silence on all tenants in examples/tenants.conf file

```
atm silence add alertname="test" --comment test-alert --duration 10m --tenant.file examples/tenants.conf
Silence added for 'tenant-a' tenant: 1fb1199b-6aec-4575-b6d4-cc5631b77326
Silence added for 'tenant-b' tenant: 0fed624d-2e62-43b8-a940-a337f40e4f05
```

## Limitations

atm couldn't view/expire silences because a silence could be created multiple time with the same matcher, so it's hard to know which silence to expire.
