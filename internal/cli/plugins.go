package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
)

func LoadPluginCommands(reg *registry.Registry, call Caller, stdin io.Reader) error {
	raw, err := call.Call("GET", "/api/plugin-contributions", map[string]string{})
	if err != nil {
		return err
	}
	var response struct {
		Plugins []plugins.Contribution `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("decode plugin contributions: %w", err)
	}
	return MergePluginCommands(reg, response.Plugins, call, stdin)
}

func MergePluginCommands(reg *registry.Registry, contributions []plugins.Contribution, call Caller, stdin io.Reader) error {
	for _, contribution := range contributions {
		if _, ok := reg.Group(contribution.Name); ok {
			return fmt.Errorf("plugin namespace %q collides with a core group", contribution.Name)
		}
		if _, ok := reg.Get(contribution.Name); ok {
			return fmt.Errorf("plugin namespace %q collides with a core command", contribution.Name)
		}
		summary := contribution.Description
		if summary == "" {
			summary = "Manage " + contribution.Name + " integration"
		}
		if err := reg.RegisterGroup(contribution.Name, summary); err != nil {
			return err
		}
		for _, command := range contribution.Commands {
			parts := strings.Split(command.Path, ".")
			for i := 1; i < len(parts); i++ {
				group := contribution.Name + "." + strings.Join(parts[:i], ".")
				if _, exists := reg.Group(group); !exists {
					if err := reg.RegisterGroup(group, strings.ReplaceAll(group, ".", " ")+" commands"); err != nil {
						return err
					}
				}
			}
			args := make([]registry.Arg, 0, len(command.Args))
			for _, arg := range command.Args {
				typ, err := pluginArgType(arg.Type)
				if err != nil {
					return err
				}
				args = append(args, registry.Arg{Name: arg.Name, Flag: arg.Flag, Type: typ, Required: arg.Required, Help: arg.Help})
			}
			declaredArgs := append([]plugins.OperatorArg(nil), command.Args...)
			pluginName, action := contribution.Name, command.Action
			if err := reg.Register(registry.Command{
				Path: pluginName + "." + command.Path, Summary: command.Summary, Args: args,
				Handler: func(_ *registry.Ctx, params registry.Params) (any, error) {
					data := map[string]any{}
					for _, arg := range declaredArgs {
						value, present := params[arg.Name]
						if !present {
							continue
						}
						if arg.Type == "secret-file" {
							secret, err := readSecretFile(value.(string), stdin)
							if err != nil {
								return nil, err
							}
							value = secret
						}
						data[arg.Name] = value
					}
					encoded, err := json.Marshal(data)
					if err != nil {
						return nil, err
					}
					raw, err := call.Call("POST", "/api/plugins/"+pluginName+"/action", map[string]any{
						"name": pluginName, "action": action, "data": string(encoded),
					})
					if err != nil {
						return nil, err
					}
					var result any
					if err := json.Unmarshal(raw, &result); err != nil {
						return nil, err
					}
					return result, nil
				},
			}); err != nil {
				return err
			}
		}
	}
	return reg.Validate()
}

func pluginArgType(typ string) (registry.ArgType, error) {
	switch typ {
	case "string":
		return registry.String, nil
	case "integer":
		return registry.Int, nil
	case "integer-list":
		return registry.IntegerList, nil
	case "boolean":
		return registry.Bool, nil
	case "secret-file":
		return registry.SecretFile, nil
	default:
		return "", fmt.Errorf("unsupported plugin argument type %q", typ)
	}
}

func readSecretFile(path string, stdin io.Reader) (string, error) {
	var data []byte
	if path == "-" {
		var err error
		data, err = io.ReadAll(io.LimitReader(stdin, 64*1024+1))
		if err != nil {
			return "", err
		}
	} else {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("secret file must be regular and owner-only (0600)")
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, 64*1024+1))
		if err != nil {
			return "", err
		}
	}
	if len(data) > 64*1024 {
		return "", fmt.Errorf("secret file is too large")
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return "", fmt.Errorf("secret file is empty")
	}
	return value, nil
}
