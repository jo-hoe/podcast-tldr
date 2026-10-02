module github.com/jo-hoe/rss-audio-downloader

go 1.26.0

require (
	github.com/jo-hoe/manifest-lib v0.0.0
	github.com/mmcdole/gofeed v1.5.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/mmcdole/goxpp/v2 v2.0.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

// The manifest library is developed alongside this repo. In CI/release it is
// consumed as a tagged module; locally it is resolved from the sibling checkout.
replace github.com/jo-hoe/manifest-lib => ../../libs/manifest-lib
