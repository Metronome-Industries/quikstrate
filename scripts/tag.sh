#!/bin/bash

scripts_dir=$(dirname $0)

function main() {
  # TODO check if on main
  # TODO check if $CI
  curr_tag=$(git describe || echo "0.0.0")

  echo "Current tag: $curr_tag"

  # TODO if commit message includes [major]
  # new_tag=$($scripts_dir/increment_version.sh -M $curr_tag)

  # TODO if commit message includes [minor]
  # new_tag=$($scripts_dir/increment_version.sh -m $curr_tag)

  new_tag=$($scripts_dir/increment_version.sh -p $curr_tag) 

  echo "New tag: $new_tag"

  git tag $new_tag
  git push origin $new_tag
}

main