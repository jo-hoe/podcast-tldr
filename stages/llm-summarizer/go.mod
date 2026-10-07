module github.com/jo-hoe/llm-summarizer

go 1.26.0

require (
	github.com/jo-hoe/manifest-lib v0.0.0
	github.com/sashabaranov/go-openai v1.43.0
	gopkg.in/yaml.v3 v3.0.1
)

// The manifest library is developed alongside this repo. In CI/release it is
// consumed as a tagged module; locally it is resolved from the sibling checkout.
replace github.com/jo-hoe/manifest-lib => ../../libs/manifest-lib
