## Docs

Kubernetes Custom Resources can be validated by the API server during creation and updates. Validation is based on the OpenAPI v3 schema specified in the validation fields in the CRD specifications.

With some adjustments, the same schema can be processed by a tool like [Redoc](https://github.com/Redocly/redoc) to validate and provide a nice documentation to the users. 

Run Redoc with Docker in the terminal: `docker run --rm --name redoc -d -p 8081:80 -e SPEC_URL='/node.yaml' -v $(pwd)/node.yaml:/usr/share/nginx/html/node.yaml redocly/redoc`

Open the url `http://localhost:8081/` with the browser.