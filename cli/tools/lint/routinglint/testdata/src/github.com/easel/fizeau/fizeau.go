package fizeau

// ServiceExecuteRequest is the minimal fixture type needed for the
// routinglint structural checks.
type ServiceExecuteRequest struct {
	Harness  string
	Provider string
	Model    string
}
