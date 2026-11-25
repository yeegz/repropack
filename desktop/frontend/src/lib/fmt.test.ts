import { describe, it, expect } from "vitest";
import { rel, duration, bytes, metricValue, valueText } from "./fmt";

describe("fmt", () => {
  it("formats relative offsets", () => {
    const t = "2026-11-25T10:00:00.000Z";
    expect(rel(t, "2026-11-25T10:00:01.500Z")).toBe("0:01.500");
    expect(rel(t, "2026-11-25T11:01:00.000Z")).toBe("1:01:00.000");
    expect(rel(t, "2026-11-25T09:59:59.000Z")).toBe("−0:01.000");
  });
  it("formats durations from nanoseconds", () => {
    expect(duration(250e6)).toBe("250 ms");
    expect(duration(7.7e9)).toBe("7.7 s");
    expect(duration(125e9)).toBe("2 min 5 s");
  });
  it("formats sizes and metrics", () => {
    expect(bytes(512)).toBe("512 B");
    expect(bytes(5 * 1024 * 1024)).toBe("5.0 MB");
    expect(metricValue(96.4, "%")).toBe("96%");
    expect(metricValue(3, "{goroutine}")).toBe("3");
  });
  it("renders values as text, never markup", () => {
    expect(valueText({ a: "<b>" })).toBe('{"a":"<b>"}');
  });
});
