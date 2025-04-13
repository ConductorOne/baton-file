#!/bin/bash
source ./.env

# To run in continuous service mode client-id and client-secret flags must be passed during execution
# ./bin/baton-file --input templates/template.yaml --client-id "${BATON_CLIENT_ID}" --client-secret "${BATON_CLIENT_SECRET}"
./bin/baton-file --input templates/template.json --client-id "${BATON_CLIENT_ID}" --client-secret "${BATON_CLIENT_SECRET}"
# ./bin/baton-file --input templates/template.xlsx --client-id "${BATON_CLIENT_ID}" --client-secret "${BATON_CLIENT_SECRET}"
