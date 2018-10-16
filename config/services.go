/*
Sniperkit-Bot
- Status: analyzed
*/

// +build !js

package config

import (
	"time"

	"github.com/dave/services/deployer"
	"github.com/dave/services/fetcher/gitfetcher"
)

var GitFetcherConfig = gitfetcher.Config{
	GitSaveTimeout:  time.Second * 300,
	GitCloneTimeout: time.Second * 300,
	GitMaxObjects:   250000,
	GitBucket:       Bucket[Git],
}

var DeployerConfig = deployer.Config{
	ConcurrentStorageUploads: ConcurrentStorageUploads,
	IndexBucket:              Bucket[Index],
	PkgBucket:                Bucket[Pkg],
	PkgProtocol:              Protocol[Pkg],
	PkgHost:                  Host[Pkg],
}
