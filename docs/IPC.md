# IPC Interaction walk through

This is a walk through for what happens durring a Chassis.Reset action.

## Backend
Subscribe to request messages
  - psubscribe __keyspace@0__:/redfish/v1/Chassis/*/Actions/Chassis.Reset

## Client
Client performs Chassis.Reset action
```
curl --header -H "Content-Type: application/json" \
  --request POST \
  --data '{ "ResetType": "On" }' \
  http://192.168.56.101:8082/redfish/v1/Chassis/APC/Actions/Chassis.Reset
```

## Frontend
- Generate Id   
- Subscribe to the IPC response key before attempting to signal the backend
  - `subscribe __keyspace__@0__:Cray:rfdispatcher:IPC:ID:Response`
- Set the request hash to a temporary location. The request key also contains the location where the backend should place its response.
  - `hmset Cray:rfdispatcher:IPC:ID:Request ResetType On ResponseKey Cray:rfdispatcher:IPC:ID:Response`
- Rename the request hash to the target key if it does not exist
  - `renamenx /redfish/v1/Chassis/APC/Actions/Chassis.Reset/Target`
  - Reply:
    - 1 - Key was renamed, the backend had been signaled with our request
    - 0 - Key was not set, it already exists. The backend is currently performing an operation.
        Either block and wait for the keys deletion, or return error.

- If key not set return error back to the client

- Wait for the backend to insert its response.

## Backend
- Receive request message from pattern subscribe
- Retrieve request
  - `hmgetall /redfish/v1/Chassis/APC/Actions/Chassis.Reset/Target`
- Process the request
- Set response using the key provided from the front end
  - `hmset Cray:rfdispatcher:IPC:ID:Response Result Success`
- Delete target key for new Chassis.Reset actions to be sent
  - `del /redfish/v1/Chassis/APC/Actions/Chassis.Reset/Target` 

## Frontend

- When the response hash is set the front end is woken backup, and retrieves the response from the backend
  - hmgetall Cray:rfdispatcher:IPC:ID:Response
- Examine the response.
  - If the field Result contains "Successful" then the action completed successfully
  - Otherwise an error occurred
- Clean up request, response
  - del Cray:rfdispatcher:IPC:ID:Request Cray:rfdispatcher:IPC:ID:Response
