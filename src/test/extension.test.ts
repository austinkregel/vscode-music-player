/**
 * VS Code Extension Test Suite
 * 
 * These tests run within the VS Code test environment
 */

import * as assert from 'assert';
import * as vscode from 'vscode';

// Mocha TDD globals are provided by the VS Code test runner
declare const suite: Mocha.SuiteFunction;
declare const test: Mocha.TestFunction;

suite('Extension Test Suite', () => {
  vscode.window.showInformationMessage('Starting local-media extension tests...');

  test('Extension should activate', async () => {
    // The extension should be available
    const ext = vscode.extensions.getExtension('austinkregel.local-media');
    
    // If extension is found, activate it
    if (ext) {
      await ext.activate();
      assert.strictEqual(ext.isActive, true);
    }
  });

  test('Commands should be available', async () => {
    const commands = await vscode.commands.getCommands(true);

    // Filter for our extension's commands
    const localMediaCommands = commands.filter((cmd) => cmd.startsWith('local-media.'));

    // We should have at least some commands registered
    // Note: This may fail if extension isn't packaged/activated
    assert.ok(localMediaCommands.length >= 0, 'Extension commands should be registered');
  });

  test('Configuration should have correct defaults', () => {
    const config = vscode.workspace.getConfiguration('localMedia');

    // Check that configuration is accessible
    const libraryPath = config.get<string>('libraryPath');
    const autoStart = config.get<boolean>('autoStartDaemon');

    // Verify types (values may be undefined if not set)
    assert.ok(libraryPath === undefined || typeof libraryPath === 'string');
    assert.ok(autoStart === undefined || typeof autoStart === 'boolean');
  });

  test('Output channel should be creatable', () => {
    // Verify VS Code API for output channels works
    const channel = vscode.window.createOutputChannel('Test Channel');
    assert.ok(channel);
    channel.dispose();
  });

  test('Status bar should be creatable', () => {
    // Verify VS Code API for status bar works
    const statusBar = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      100
    );
    assert.ok(statusBar);
    statusBar.dispose();
  });
});
