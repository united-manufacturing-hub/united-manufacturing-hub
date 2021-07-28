docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli validate -i /local/factoryinsight.yml
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g python -i /local/factoryinsight.yml -o /local/output/python
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g go -i /local/factoryinsight.yml -o /local/output/go
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g markdown -i /local/factoryinsight.yml -o /local/output/markdown
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g rust -i /local/factoryinsight.yml -o /local/output/rust
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g html2 --generate-alias-as-model  -i /local/factoryinsight.yml -o /local/output/html