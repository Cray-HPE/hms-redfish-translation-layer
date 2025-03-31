# Redfish Translation Service

This is a Redfish Server that:
1. Walks an in-memory version of the Redfish schema tree to see if a property exist
2. Queries Redis for a list of resource properties
	* Checks to see if the property exist in redis
    * Either returns the redis property, or executes a back-end handler
3. Uses Redfish property type (number, string, array) to build a struct/JSON object
4. Delivers JSON object (or error) to user

TODO: Describe input verbs, which are similar but ordered a bit differently

## Key design principles (in order of importance):
 * Decompose redfish tree -> to a flat redis DB programmatically
 * Dispatch handlers directly (vs. relying on another dipsatch agent w/ handshaking)
 * Use consistent and unprocessed URL -> Redis Key -> handler definitions, so relationships/flow is obvious
 * Avoid unnecessary redis meta-data

### Prototype Example:
Redfish URI:
```
http://sms1.rbt1.next.cray.com:8082/redfish/v1/Chassis/APC
```
Redis representation:
```
redis /var/run/redis/redis.sock> SMEMBERS /redfish/v1/Chassis/APC
 1) "Actions"
 2) "Id"
 3) "PowerState"
 4) "Manufacturer"
 5) "@odata.type"
 6) "SerialNumber"
 7) "@odata.context"
 8) "Power"
 9) "@odata.id"
10) "Model"
11) "ChassisType"
12) "Name"

redis /var/run/redis/redis.sock> get /redfish/v1/Chassis/APC/ChassisType
"RackMount"
```
back-end handler script:
```
seanb@sms1.rbt1:~ $ ls /chfs/pdu/redfish/v1/Chassis/APC/PowerState
/chfs/pdu/redfish/v1/Chassis/APC/PowerState
```

## Getting Started

### Using Docker Compose

The easiest way to stand up this entire framework in simulator fashion along with all of the other necessary HMS software is to use Docker compose.

First generate the required HTTPS certificate:

```bash
openssl req -x509 -nodes -newkey rsa:4096 -keyout configs/rts.key -out configs/rts.crt -sha256 -days 1 \
        -subj "/C=US/O=RTS/OU=TEST_CERTIFICATE/CN=localhost"
```

And set permissions so that rts.key can be referenced:
```bash
chmod go+r configs/rts.key
```

Then stand up the everything:

```text
$ docker-compose -f docker-compose.simulator.yaml up -d
Starting hms-redfish-translation-service_redis_1                ... done
Starting hms-redfish-translation-service_zookeeper_1            ... done
Starting hms-redfish-translation-service_cray-hms-hmcollector_1 ... done
Starting hms-redfish-translation-service_vault_1                ... done
Starting hms-redfish-translation-service_hmsds-postgres_1       ... done
Starting hms-redfish-translation-service_kafka_1                ... done
Starting hms-redfish-translation-service_vault-kv-enabler_1     ... done
Starting hms-redfish-translation-service_cray-smd-init_1        ... done
Starting hms-redfish-translation-service_cray-smd_1             ... done
Starting hms-redfish-translation-service_redfish-simulator_1    ... done
```

After a little time and the services have settled you should see all services running with their respective ports exposed:

```text
$ docker container ls
CONTAINER ID        IMAGE                                               COMMAND                  CREATED             STATUS              PORTS                                        NAMES
e293adcb1a5a        hms-redfish-translation-service_redfish-simulator   "/docker-entrypoint.…"   19 minutes ago      Up 6 seconds        0.0.0.0:8082-8083->8082-8083/tcp             hms-redfish-translation-service_redfish-simulator_1
f1a0a82cd18f        dtr.dev.cray.com/cray/cray-smd                      "sh -c 'smd -db-dsn=…"   19 minutes ago      Up 6 seconds        0.0.0.0:27779->27779/tcp                     hms-redfish-translation-service_cray-smd_1
6c5206718ccc        confluentinc/cp-kafka:5.4.0                         "/etc/confluent/dock…"   19 minutes ago      Up 7 seconds        0.0.0.0:9092->9092/tcp                       hms-redfish-translation-service_kafka_1
07d5e915bbac        dtr.dev.cray.com/cray/hms-hmcollector               "sh -c hmcollector"      19 minutes ago      Up 7 seconds        80/tcp                                       hms-redfish-translation-service_cray-hms-hmcollector_1
92414a0b9bf7        confluentinc/cp-zookeeper:5.4.0                     "/etc/confluent/dock…"   19 minutes ago      Up 7 seconds        2888/tcp, 0.0.0.0:2181->2181/tcp, 3888/tcp   hms-redfish-translation-service_zookeeper_1
9cee405ca7e6        postgres                                            "docker-entrypoint.s…"   19 minutes ago      Up 7 seconds        0.0.0.0:5432->5432/tcp                       hms-redfish-translation-service_hmsds-postgres_1
3a18c54258c5        redis:alpine                                        "docker-entrypoint.s…"   19 minutes ago      Up 7 seconds        0.0.0.0:6379->6379/tcp                       hms-redfish-translation-service_redis_1
0facb4a271ce        vault                                               "docker-entrypoint.s…"   19 minutes ago      Up 7 seconds        0.0.0.0:8200->8200/tcp                       hms-redfish-translation-service_vault_1
```

You can even query HSM to see all of the endpoints that RTS is simulating discovered as you would expect:

```text
$ curl -L -X GET 'http://localhost:27779/hsm/v2/Inventory/RedfishEndpoints' | jq
{
  "RedfishEndpoints": [
    {
      "ID": "x0c0s4b0",
      "Type": "NodeBMC",
      "Hostname": "x0c0s4b0:8083",
      "Domain": "",
      "FQDN": "x0c0s4b0:8083",
      "Enabled": true,
      "User": "root",
      "Password": "",
      "RediscoverOnUpdate": true,
      "DiscoveryInfo": {
        "LastDiscoveryAttempt": "2020-04-16T16:10:34.869336Z",
        "LastDiscoveryStatus": "DiscoverOK",
        "RedfishVersion": "2019.1"
      }
    },
    {
      "ID": "x0c0s2b0",
      "Type": "NodeBMC",
      "Hostname": "x0c0s2b0:8083",
      "Domain": "",
      "FQDN": "x0c0s2b0:8083",
      "Enabled": true,
      "User": "root",
      "Password": "",
      "RediscoverOnUpdate": true,
      "DiscoveryInfo": {
        "LastDiscoveryAttempt": "2020-04-16T16:10:34.839511Z",
        "LastDiscoveryStatus": "DiscoverOK",
        "RedfishVersion": "2019.1"
      }
    },
    {
      "ID": "x0c0s1b0",
      "Type": "NodeBMC",
      "Hostname": "x0c0s1b0:8083",
      "Domain": "",
      "FQDN": "x0c0s1b0:8083",
      "Enabled": true,
      "User": "root",
      "Password": "",
      "RediscoverOnUpdate": true,
      "DiscoveryInfo": {
        "LastDiscoveryAttempt": "2020-04-16T16:10:34.828303Z",
        "LastDiscoveryStatus": "DiscoverOK",
        "RedfishVersion": "2019.1"
      }
    },
    {
      "ID": "x0c0s3b0",
      "Type": "NodeBMC",
      "Hostname": "x0c0s3b0:8083",
      "Domain": "",
      "FQDN": "x0c0s3b0:8083",
      "Enabled": true,
      "User": "root",
      "Password": "",
      "RediscoverOnUpdate": true,
      "DiscoveryInfo": {
        "LastDiscoveryAttempt": "2020-04-16T16:10:34.856080Z",
        "LastDiscoveryStatus": "DiscoverOK",
        "RedfishVersion": "2019.1"
      }
    }
  ]
}
```

Vault is also accessible via a web interface at [http://localhost:8200/](http://localhost:8200/) where you can login using the token `hms`.

If you want to talk directly to the endpoints you will need to modify your local `/etc/hosts` file to have the xnames defined to resolve to your local computer. RTS works by looking at the host information passed in to determine the xname of the simulated endpoint you're trying to talk to. As such, you can't just use `localhost` becuase RTS wouldn't be able to tell which piece of hardware you actually meant.

If you're using the configuration as it comes, add this line to `/etc/hosts` should be all that's required:

```text
127.0.0.1	x0c0s1b0 x0c0s2b0 x0c0s3b0 x0c0s4b0
```

Now you can query the xname directly:

```text
$ curl -H 'Authorization: Basic cm9vdDppbml0aWFsMA==' -k -L -X GET 'https://x0c0s1b0:8083/redfish/v1/Systems/Self' | jq
{
  "@odata.id": "/redfish/v1/Systems/Self",
  "@odata.type": "#ComputerSystem.v1_7_0.ComputerSystem",
  "Actions": {
    "#ComputerSystem.Reset": {
      "ResetType@Redfish.AllowableValues": [
        "On",
        "GracefulShutdown",
        "ForceRestart"
      ],
      "target": "/redfish/v1/Systems/Self/Actions/ComputerSystem.Reset"
    }
  },
  "Id": "Self",
  "Manufacturer": "Cray, Inc.",
  "MemorySummary": {
    "TotalSystemMemoryGiB": 42
  },
  "Model": "Redfish Simulator",
  "Name": "Computer System Chassis",
  "PowerState": "On",
  "ProcessorSummary": {
    "Count": 10,
    "Model": "Simulated"
  }
}
```

You can even tell the node to turn off:

```text
$ curl -L -X POST 'http://x0c0s1b0:8082/redfish/v1/Systems/Self/Actions/ComputerSystem.Reset' \
-H 'Authorization: Basic cm9vdDppbml0aWFsMA==' -H 'Content-Type: application/json' --data-raw '{
        "ResetType": "GracefulShutdown"
}'

$ curl -H 'Authorization: Basic cm9vdDppbml0aWFsMA==' -k -L -X GET 'https://x0c0s1b0:8083/redfish/v1/Systems/Self' | jq '.PowerState'
"Off"
```


### Running Locally

If you want to run this service locally the first step is to set your `GOPATH` to the `go` folder at the root of this
project. Then you can `go build rfdispatcher` to get the executable.

To run the RTS you need to set at least 3 environment variables:

```bash
SCHEMA_VERSION=2018.3
REDIS_ADDR=localhost:6379
PRIVILEGE_REGISTRY_VERSION=1.0.3
```

In addition, all of the files consumed by this service live in the `contrib` directory and if you run the RTS outside 
of the context of this directory you will need to set the `CONTRIB_DIR_PREFIX` to a value just above the `contrib` 
directory.

## Running the tests

TODO: Explain how to run the automated tests for this system

### Break down into end to end tests

Explain what these tests test and why:
TODO: Though because this design relies on a programatic method of using redis keys and handlers it becomes possible to:
* Generated ALL keys for insertion, enabling 100% redfish server test coverage
* Generate a list of all back-end handers, so 100% of back-ends can be exercised
* Test (on the fly) PropertyType, Permissions, NullAllowed, Deprecated, Enum validity, required fields, pattern properties, Min/Max/Format

```
TODO: Give an example
```

### And coding style tests

TODO: Explain what these tests test and why

```
TODO: Give an example
```

## Deployment

TODO: Add additional notes about how to deploy this on a live system

## Built With

* [go-redis](https://github.com/go-redis/redis) - The redis client used

## Versioning

TODO

## License

TODO

## Acknowledgments

* This Redfish implementation heavily utilizes Ryan Sjostrand's redfish parsing framework, which does much of the work and works very well.
* This design leverages ideas and time (typically untracked and verbal) from many people on the ES and HMS teams, including, but not limited to: Sean Wallace, Rob Zirkel, David Rush, Benjamin Gilsrud, Steve Martin and Rod Frost.

