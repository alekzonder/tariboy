const daemonURL = "http://127.0.0.1:4176";
let callbackID = 0;
let eventID = 0;

type TauriInternals = {
  invoke: (command: string) => Promise<unknown>;
  transformCallback: () => number;
  unregisterCallback: () => void;
  convertFileSrc: (path: string) => string;
};

const internals: TauriInternals = {
  async invoke(command) {
    switch (command) {
      case "daemon_status":
        return {
          state: "ready",
          base_url: daemonURL,
          daemon_version: "e2e",
          app_version: "e2e",
          base_dir: "/isolated/tasks-e2e",
          pid: 1,
          adopted: false,
          message: "",
        };
      case "hosts_list":
        return [];
      case "plugin:event|listen":
        eventID += 1;
        return eventID;
      case "plugin:event|unlisten":
        return null;
      default:
        throw new Error(`unexpected Tauri command in Tasks E2E: ${command}`);
    }
  },
  transformCallback() {
    callbackID += 1;
    return callbackID;
  },
  unregisterCallback() {},
  convertFileSrc(path) {
    return path;
  },
};

Object.assign(window, {
  __TAURI_INTERNALS__: internals,
  __TAURI_EVENT_PLUGIN_INTERNALS__: {
    unregisterListener() {},
  },
});

await import("../src/main");
