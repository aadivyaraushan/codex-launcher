import fs from 'node:fs';
import path from 'node:path';
import {randomBytes} from 'node:crypto';

function hasWorkspaceData(directory) {
  try { return fs.statSync(directory).isDirectory() && fs.readdirSync(directory).length > 0; }
  catch { return false; }
}

function relocateManagedWorkspace(state, configPath) {
  const source = fs.readFileSync(configPath, 'utf8');
  let config;
  try { config = JSON.parse(source); }
  catch { return; }
  const configured = config?.agents?.defaults?.workspace;
  const workspace = path.join(state, 'workspace');
  const managedWorkspace = typeof configured === 'string' && path.isAbsolute(configured) && path.basename(configured) === 'workspace' && path.basename(path.dirname(configured)) === 'openclaw' && path.basename(path.dirname(path.dirname(configured))) === 'Operator';
  if (!managedWorkspace || configured === workspace) return;
  if (!hasWorkspaceData(workspace)) {
    if (!hasWorkspaceData(configured)) throw new Error('The app-managed workspace is missing. Restore it before reopening the local runtime.');
    // Preserve the old container until the owner chooses when it is safe to remove.
    fs.cpSync(configured, workspace, {recursive: true, errorOnExist: true, force: false});
  }
  delete config.agents.defaults.workspace;
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), {mode: 0o600});
}

function addAutomaticFastModeDefault(configPath) {
  let config;
  try { config = JSON.parse(fs.readFileSync(configPath, 'utf8')); }
  catch (error) { if (error instanceof SyntaxError) return; throw error; }
  const isObject = value => value !== null && typeof value === 'object' && !Array.isArray(value);
  if (!isObject(config) || (config.agents !== undefined && !isObject(config.agents))) return;
  if (config.agents?.defaults !== undefined && !isObject(config.agents.defaults)) return;
  if (config.agents?.defaults?.fastModeDefault !== undefined) return;
  config.agents ??= {};
  config.agents.defaults ??= {};
  config.agents.defaults.fastModeDefault = 'auto';
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), {mode: 0o600});
}

function addNativeSearchDefaults(configPath) {
  let config;
  try { config = JSON.parse(fs.readFileSync(configPath, 'utf8')); }
  catch (error) { if (error instanceof SyntaxError) return; throw error; }
  const isObject = value => value !== null && typeof value === 'object' && !Array.isArray(value);
  if (!isObject(config)) return;
  let current = config;
  for (const key of ['tools', 'web', 'search', 'openaiCodex']) {
    if (current[key] !== undefined && !isObject(current[key])) return;
    current = current[key] ?? {};
  }
  const search = config.tools?.web?.search;
  // Never replace a chosen provider, disable, mode or search restriction.
  if (search?.enabled === false || search?.provider !== undefined || search?.openaiCodex?.enabled === false) return;
  if (current.enabled !== undefined && current.mode !== undefined) return;
  config.tools ??= {};
  config.tools.web ??= {};
  config.tools.web.search ??= {};
  config.tools.web.search.openaiCodex ??= {};
  const native = config.tools.web.search.openaiCodex;
  if (native.enabled === undefined) native.enabled = true;
  if (native.mode === undefined) native.mode = 'live';
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), {mode: 0o600});
}

export function prepareState(state) {
  fs.mkdirSync(state, {recursive: true, mode: 0o700});
  const workspace = path.join(state, 'workspace');
  const configPath = path.join(state, 'openclaw.json');
  const config = {
    gateway: {mode: 'local', bind: 'loopback', auth: {mode: 'token', token: randomBytes(32).toString('hex')}, controlUi: {enabled: false}},
    // OpenClaw resolves the default workspace from OPENCLAW_STATE_DIR/workspace.
    // Keeping it out of the file survives an iOS app-container relocation.
    agents: {defaults: {fastModeDefault: 'auto'}},
    tools: {web: {search: {openaiCodex: {enabled: true, mode: 'live'}}}}
  };
  try {
    // Exclusive creation: reopening must never replace settings or saved sign-in.
    fs.writeFileSync(configPath, JSON.stringify(config), {flag: 'wx', mode: 0o600});
    fs.mkdirSync(workspace, {recursive: true, mode: 0o700});
    return {configPath, created: true};
  } catch (error) {
    if (error.code !== 'EEXIST') throw error;
    // Only migrate the app's former workspace location after its contents survive.
    // Other JSON5 configuration remains OpenClaw-owned and is left untouched.
    relocateManagedWorkspace(state, configPath);
    // A saved default keeps ordinary chat free of restart-unsafe message overrides.
    // Preserve explicit preferences and leave non-JSON configuration untouched.
    addAutomaticFastModeDefault(configPath);
    addNativeSearchDefaults(configPath);
    fs.mkdirSync(workspace, {recursive: true, mode: 0o700});
    return {configPath, created: false};
  }
}
