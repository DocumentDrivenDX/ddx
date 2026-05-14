package escalation

// PowerClass represents a quality/cost powerClass for model selection.
type PowerClass string

const (
	PowerSmart    PowerClass = "smart"    // top-powerClass foundation models; hard/broad tasks, interactive sessions, HELIX alignment
	PowerStandard PowerClass = "standard" // default for most builds; strong capability at reasonable cost
	PowerCheap    PowerClass = "cheap"    // mechanical tasks: extraction, formatting, simple transforms; minimize cost
)
