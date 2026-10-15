/**
 * Platform tool approval gate.
 *
 * pi's core never asks before running a tool. This extension bridges that gap
 * for the platform's RPC transport: it turns a tool call into a `ctx.ui.confirm`
 * dialog, which the adapter forwards to the workbench as a permission card.
 * The workbench's answer comes back as an extension UI response.
 *
 * The platform installs this extension automatically for RPC-mode sessions.
 * It confirms commands and edits that can change or expose the workspace;
 * read-only calls pass through.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// Bash commands dangerous enough to always require approval.
const dangerousBashPatterns = [
	/\brm\s+(-[a-z]*[rf][a-z]*|--recursive|--force)/i,
	/\bsudo\b/i,
	/\b(chmod|chown)\b.*777/i,
	/\bgit\s+(reset\s+--hard|clean\s+-[a-z]*f)/i,
	/\bdd\s+if=/i,
	/\bmkfs\b/i,
	/:\(\)\s*\{.*\};\s*:/,
];

function summarizeBash(input: Record<string, unknown>): string {
	const command = typeof input.command === "string" ? input.command : JSON.stringify(input);
	return command.length > 500 ? `${command.slice(0, 500)}…` : command;
}

function summarizeEdit(toolName: string, input: Record<string, unknown>): string {
	if (typeof input.path === "string") return `${toolName} ${input.path}`;
	return `${toolName} ${JSON.stringify(input).slice(0, 300)}`;
}

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", async (event, ctx) => {
		if (!ctx.hasUI) return undefined;

		const input = (event.input ?? {}) as Record<string, unknown>;
		let needsApproval = false;
		let detail = "";

		switch (event.toolName) {
			case "bash": {
				const command = typeof input.command === "string" ? input.command : "";
				needsApproval = dangerousBashPatterns.some((pattern) => pattern.test(command));
				detail = summarizeBash(input);
				break;
			}
			case "write":
			case "edit":
			case "patch":
				needsApproval = true;
				detail = summarizeEdit(event.toolName, input);
				break;
			default:
				needsApproval = false;
		}

		if (!needsApproval) return undefined;

		const confirmed = await ctx.ui.confirm(`Allow ${event.toolName}?`, detail);
		if (!confirmed) {
			ctx.ui.notify(`${event.toolName} blocked by user`, "warning");
			return { block: true, reason: `${event.toolName} was not approved` };
		}
		return undefined;
	});
}
