#!/bin/bash

schema_dir="../../../../ext/secapi/spec/schemas"
echo "Generating models from OpenAPI schemas in $schema_dir"

files=$(ls $schema_dir/*.yaml)
basenames=$(basename -a $files)

import_mappings=""
for base in $basenames; do
  import_mappings+="  ./$base: \"-\""$'\n'
done

for base in $basenames; do
  cat <<EOF > ./config.yaml
package: models
output: ${base%.yaml}.gen.go
output-options:
  skip-prune: true
import-mapping:
$import_mappings
generate:
  models: true
EOF
  
    echo "Generating model for $base"
    go tool oapi-codegen -config ./config.yaml $schema_dir/$base
    rm -rf config.yaml
done
