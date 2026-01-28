#!/bin/bash

spec_dir="../../../ext/secapi/spec/"
schema_dir="../../../ext/secapi/spec/schemas"
echo "Generating apis from OpenAPI schemas in $spec_dir"

schema_files=$(ls $schema_dir/*.yaml)
schema_basenames=$(basename -a $schema_files)

files=$(ls $spec_dir/*.yaml)
basenames=$(basename -a $files)

import_mappings=""
for base in $schema_basenames; do
  import_mappings+="  ./schemas/$base: cape-project.eu/sdk-generator/provider/pulumi/secapi/models"$'\n'
done

for base in $basenames; do
  base_no_ext="${base%.yaml}"
  IFS='.' read -r -a name_parts <<< "$base_no_ext"
  folder_path="$(IFS=/; echo "${name_parts[*]}")"
  package_name="${name_parts[${#name_parts[@]}-1]}"
  output_dir="./$folder_path"
  output_file="$output_dir/${base_no_ext}.gen.go"

  mkdir -p "$output_dir"

  cat <<EOF > ./config.yaml
package: $package_name
output: $output_file
output-options:
  skip-prune: true
import-mapping:
$import_mappings
generate:
  client: true
  models: true
EOF
  
    echo "Generating model for $base"
    go tool oapi-codegen -config ./config.yaml $spec_dir/$base
    rm -rf config.yaml
done
