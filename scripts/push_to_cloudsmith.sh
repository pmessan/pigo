#!/usr/bin/env bash

pip3 install -U pip cloudsmith-cli wheel setuptools >/dev/null

VERSION=$(git describe --tag | grep "[0-9\.\-]\+[a-zA-Z0-9]\+$" -m 1 -o | head -1)

cloudsmith push raw --republish \
  --name "Pigo" \
  --version "$VERSION" \
    "proglove/gateway-tools" \
    "packages/proglove-pigo.tar.gz"
