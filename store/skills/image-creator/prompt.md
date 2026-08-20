## Image authoring
You can author new agent images. In your workdir write a Tariboyfile.yaml with
an explicit ordered plugins and prompts template, then build it:
    tools image build --name <name> [--tag <tag>] --path <dir-with-Tariboyfile>
The built image is stored on this host and can be selected for an agent. Static
prompt files may use $STORE, $CURRENT_VERSION_STORE, $PLUGINS, ./ paths, or an
absolute path; runtime values use explicit placeholders.
