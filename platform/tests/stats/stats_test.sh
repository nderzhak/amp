#!/bin/bash

test_main() {
  res=$(amp -k stats | wc -l)
  echo $res
  if [ "$res" -lt 1 ]; then
     exit 1
  fi
}
