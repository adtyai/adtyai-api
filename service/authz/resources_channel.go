package authz

const (
	ResourceChannel = "channel"

	ActionRead           = "read"
	ActionOperate        = "operate"
	ActionWrite          = "write"
	ActionSensitiveWrite = "sensitive_write"
	ActionSecretView     = "secret_view"
	ActionCreate         = "create"
)

var (
	ChannelRead           = Permission{Resource: ResourceChannel, Action: ActionRead}
	ChannelOperate        = Permission{Resource: ResourceChannel, Action: ActionOperate}
	ChannelWrite          = Permission{Resource: ResourceChannel, Action: ActionWrite}
	ChannelSensitiveWrite = Permission{Resource: ResourceChannel, Action: ActionSensitiveWrite}
	ChannelSecretView     = Permission{Resource: ResourceChannel, Action: ActionSecretView}
	ChannelCreate         = Permission{Resource: ResourceChannel, Action: ActionCreate}
	ChannelDelete         = Permission{Resource: ResourceChannel, Action: ActionDelete}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceChannel,
		LabelKey: "Channel Management",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read channels",
				DescriptionKey: "View channel lists and details without secrets.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate channels",
				DescriptionKey: "Test channels, refresh balances, and enable/disable individual, batch, or tagged channels.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit channel routing",
				DescriptionKey: "Edit non-sensitive settings such as models, groups, and routing rules.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionCreate,
				LabelKey:       "Create channels",
				DescriptionKey: "Create channels and copy existing channels.",
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit sensitive channel settings",
				DescriptionKey: "Edit channel keys, base URLs, and overrides.",
			},
			{
				Action:         ActionDelete,
				LabelKey:       "Delete channels",
				DescriptionKey: "Delete individual channels, selected channels, or disabled channels.",
			},
			{
				Action:         ActionSecretView,
				LabelKey:       "View channel secrets",
				DescriptionKey: "Reserved for viewing complete channel keys after secure verification.",
			},
		},
	})
}
