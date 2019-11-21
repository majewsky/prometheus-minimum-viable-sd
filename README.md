# prometheus-minimum-viable-sd

The Minimum-Viable service discovery plugin for [Prometheus](https://prometheus.io).

## Installation and usage

Build with `make`. Package with `make install`. The only dependency is
[a Go compiler](https://golang.org).

### On the server

This tool builds on Prometheus' native [file-based service discovery][file-sd].
Put this in your Prometheus server config:

```yaml
scrape_configs:
  - job_name: minimum-viable-sd # or any job name of your choice
    file_sd_configs:
      - files: [ '/run/prometheus/services.json' ] # or any path of your choice
```

And run the service discovery job like this:

```sh
OUTPUT_PATH=/run/prometheus/services.json # same path as in the Prometheus config
LISTEN_ADDRESS=0.0.0.0:12345              # we listen for announcements on this TCP socket
prometheus-minimum-viable-sd collect $OUTPUT_PATH $LISTEN_ADDRESS
```

The `$OUTPUT_PATH` is ephemeral and gets written to regularly, so it's a good
idea to put it on a tmpfs as in the example above.

[file-sd]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config

### On each client

On each node that runs services with Prometheus endpoints, have a JSON file like this:

```json
[
  {
    "targets": [ "192.168.1.2:9100" ],
    "labels": { "service": "node-exporter", "hostname": "foo" }
  },
  {
    "targets": [ "192.168.1.2:9559" ],
    "labels": { "service": "ntp-exporter", "hostname": "foo" }
  }
]
```

Announce those services to the Prometheus server like this:

```sh
INPUT_PATH=/etc/prometheus/services.json # the file from above
TARGET_ADDRESS=192.168.1.16:12345        # where `prometheus-minimum-viable-sd collect` is listening
prometheus-minimum-viable-sd announce $INPUT_PATH $TARGET_ADDRESS
```

The collector will just concatenate all JSON files from all announcers, so:

- Make sure that the labels are sufficiently unique to disambiguate timeseries
  submitted by different nodes and services.
- In the `targets` list, use IP addresses that identify the announcing node
  from the perspective of the Prometheus server. If you use `127.0.0.1` or
  `localhost`, this will be copied into the resulting file verbatim and the
  server will try to connect to itself rather than to the announcing node.

The announcer will keep running and resend its announcement in regular
intervals. This allows the collector to garbage-collect stale announcements
after a node has gone down. You probably want to run the announcer as a systemd
service or similar.

Failure to send an announcement (e.g. because the node hosting the collector is
unreachable) is considered a fatal error and will cause the announcer to exit
immediately with non-zero status. This ensures that you have immediate
visibility into connectivity issues between announcer and collector. Make sure
to arrange for the announcer to be restarted in such a situation, e.g. by
putting the following attributes into the systemd service:

```ini
[Service]
Restart=always
RestartSec=10s
```
