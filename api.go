package harness

import (
	"context"

	coremodel "github.com/carsonfeng/harness/model"
	coreskill "github.com/carsonfeng/harness/skill"
	corethread "github.com/carsonfeng/harness/thread"
	coretool "github.com/carsonfeng/harness/tool"
)

// Focused subpackage types are re-exported here so applications can use the
// complete SDK with one import. Adapter authors may import the subpackages.
type (
	Role           = coremodel.Role
	Message        = coremodel.Message
	ToolCall       = coremodel.ToolCall
	ToolDefinition = coremodel.ToolDefinition
	ModelRequest   = coremodel.Request
	ModelResponse  = coremodel.Response
	Model          = coremodel.Model
	ModelFunc      = coremodel.Func
	Tool           = coretool.Tool
	Skill          = coreskill.Skill
	Thread         = corethread.Thread
)

const (
	RoleSystem    = coremodel.RoleSystem
	RoleUser      = coremodel.RoleUser
	RoleAssistant = coremodel.RoleAssistant
	RoleTool      = coremodel.RoleTool
)

// Func exposes a typed Go function as a model tool.
// @param name model-visible tool name.
// @param description model-visible purpose.
// @param fn typed Go function.
// @return callable tool.
func Func[In, Out any](name, description string, fn func(context.Context, In) (Out, error)) Tool {
	return coretool.Func(name, description, fn)
}

// LoadSkills loads SKILL.md directories. Most applications use SkillDir.
// @param root directory containing skill folders.
// @return skills by name or loading error.
func LoadSkills(root string) (map[string]Skill, error) { return coreskill.LoadDir(root) }

// NewThread creates a conversation thread initialized with messages.
// @param messages initial conversation messages.
// @return new thread.
func NewThread(messages ...Message) *Thread { return corethread.New(messages...) }
