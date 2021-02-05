# Building the markdown file from the yml

Under Windows run:
`docker run -v %cd%:/app -t quay.io/verygoodsecurity/widdershins-docker --search false --environment /app/env.json --summary /app/factoryinsight.yml -o /app/factoryinsight.md`

Under Linux exchange `%cd%` with `$(pwd)`