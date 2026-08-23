import type { RunCommandStatus } from './types';

// Projection state is already the shared presentation of the exact
// Workspace-owner admission. The Web console preserves it verbatim and never
// reclassifies from caller-authored requestReason or owner diagnostic text.
export function presentWorkspaceRunCommand(command?: RunCommandStatus) {
  if (!command || (command.state !== 'accepted' && command.state !== 'rejected')) {
    return undefined;
  }
  return command;
}
