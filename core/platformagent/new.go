package platformagent

import "errors"

// Agent is the loop; it implements Runner.
type Agent struct{ Runner }

// New opens or resumes the conversation Options say.
func New(o Options) (*Agent, error) { return nil, errors.New("not built yet") }
