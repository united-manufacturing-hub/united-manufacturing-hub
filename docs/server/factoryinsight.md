TODO: #81 fill out standardized documentation for factoryinsight

# |NAME OF DOCKER CONTAINER|

|This is a short description of the docker container.|

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

|TUTORIAL|

docker-compose -f ./deployment/factoryinsight/docker-compose-factoryinsight-development.yml --env-file ./.env up -d --build

## Environment variables

This chapter explains all used environment variables.

### |EXAMPLE_VARIABLE|

Description: |DESCRIPTION|

Type: |BOOL, STRING, etc.|

Possible values: |if restricted|

Example value: |EXAMPLE|