# Local Testing

## JAWS backend testing

1. Copy `configs/vault_loader.template.env` to `configs/vault_loader.env`.
2. Edit `configs/vault_loader.env` to contain PDU credentials.

    Example
    ```bash
    VAULT_RTS_DEFAULT_CREDENTIALS='{"Username": "root", "Password": "rts"}'
    VAULT_PDU_DEFAULT_CREDENTIALS='{"Username": "admn", "Password": "foo"}'
    ```

3. Copy `configs/iPDUs.template.json` to `configs/iPDUs.json`.
4. Edit `configs/iPDUs.json` to contain information about the PDU:
    
    Example `iPDUs.json` pointing toward the Fake PDU defined in `docker-compose.jaws.yaml`:
    ```json
    {
        "Devices": [
            {
                "password": "foo",
                "url": "http://fake-pdus:8090/jaws",
                "username": "admn",
                "xname": "x0m0"
            }
        ]
    }
    ```

5. Standup RTS and related services:

    ```bash
    docker-compose -f docker-compose.jaws.yaml up --build
    ```

6. Add PDUs into vault:

    ```bash
    ./scripts/runPDULoader.sh
    ```

## Mock backend testing
This will run RTS locally instead of within a docker container.

1. Generate HTTPs Certificate:
    ```bash
    openssl req -x509 -nodes -newkey rsa:4096 -keyout configs/rts.key -out configs/rts.crt -sha256 -days 1 \
        -subj "/C=US/O=RTS/OU=TEST_CERTIFICATE/CN=localhost"
    ```

2. Stand up dependencies:
    ```bash
    docker-compose -f docker-compose.devel.yaml up --build 
    ```

3. Start RTS:
    ```bash
    ./scripts/runRTS.sh
    ```
