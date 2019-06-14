# Automium

[![Build Status](https://travis-ci.org/automium/automium.svg?branch=master)](https://travis-ci.org/automium/automium)
[![Docker Hub](https://img.shields.io/badge/docker-ready-blue.svg?style=flat-square)](https://hub.docker.com/r/automium/automium/)

## Usage

Install the CRDs and deploy the manager into the target cluster:  

```
make install

make deploy
```

## Getting started

Automium should be run in production, in a HA Kubernetes Cluster. To contribute to Automium development or just playing around, once you have the code downloaded, follow the **[Setup guide](helm/SETUP.md)** to get started. 