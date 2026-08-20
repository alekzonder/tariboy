import { describe, expect, it } from "vitest"
import { hostToParam, paramToHost, serverPath, targetFor } from "./terminalsHost"

describe("terminals host param mapping", () => {
  it("round-trips local", () => {
    expect(hostToParam("")).toBe("local")
    expect(paramToHost("local")).toBe("")
  })
  it("round-trips a registry id", () => {
    expect(hostToParam("d_abc")).toBe("d_abc")
    expect(paramToHost("d_abc")).toBe("d_abc")
  })

  it("builds explicit server workspace paths", () => {
    expect(serverPath("", "tasks")).toBe("/servers/local/tasks")
    expect(serverPath("remote 1", "images")).toBe("/servers/remote%201/images")
  })

  it("fails closed when a remote host is not in the session cache", () => {
    expect(targetFor("missing-host")).toEqual({
      id: "missing-host",
      label: "missing-host",
      baseURL: "",
      token: "",
    })
  })
})
