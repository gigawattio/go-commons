# go-commons

[![Documentation](https://godoc.org/github.com/gigawattio/go-commons?status.svg)](https://godoc.org/github.com/gigawattio/go-commons)
[![Build Status](https://travis-ci.org/gigawattio/go-commons.svg?branch=master)](https://travis-ci.org/gigawattio/go-commons)
[![Report Card](https://goreportcard.com/badge/github.com/gigawattio/go-commons)](https://goreportcard.com/report/github.com/gigawattio/go-commons)

### Shared go packages used by projects at [Gigawatt](https://gigawatt.io/).

Formerly known as `gigawatt-common'.

### Requirements

* Go version 1.6 or newer
* Locally running postgres database for running the unit-tests.

### Running the test suite

    go test ./...

## TODO

* [ ] Fix upstart serviceifier to pickup and preserve flags.
* [ ] Update upstart serviceifier refuse to install as root?

