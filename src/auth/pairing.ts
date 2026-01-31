/**
 * Pairing UX flow
 * Handles the authentication flow between extension and daemon
 */

import * as vscode from 'vscode';
import { IPCClient } from '../daemon/client';
import type { PairResponse } from '../types';

const SECRET_KEY = 'musicd-token';
const CLIENT_NAME = 'VS Code Extension';

/**
 * Manages authentication with the daemon
 */
export class AuthManager {
  private client: IPCClient;
  private secrets: vscode.SecretStorage;
  private _isAuthenticated: boolean = false;

  constructor(client: IPCClient, secrets: vscode.SecretStorage) {
    this.client = client;
    this.secrets = secrets;
  }

  /**
   * Check if we have a stored token
   */
  async hasStoredToken(): Promise<boolean> {
    const token = await this.secrets.get(SECRET_KEY);
    return token !== undefined && token.length > 0;
  }

  /**
   * Get the stored token
   */
  async getStoredToken(): Promise<string | undefined> {
    return this.secrets.get(SECRET_KEY);
  }

  /**
   * Store the authentication token
   */
  async storeToken(token: string): Promise<void> {
    await this.secrets.store(SECRET_KEY, token);
    this.client.setToken(token);
    this._isAuthenticated = true;
  }

  /**
   * Clear the stored token
   */
  async clearToken(): Promise<void> {
    await this.secrets.delete(SECRET_KEY);
    this._isAuthenticated = false;
  }

  /**
   * Check if currently authenticated
   */
  get isAuthenticated(): boolean {
    return this._isAuthenticated;
  }

  /**
   * Try to authenticate with a stored token
   */
  async tryStoredToken(): Promise<boolean> {
    const token = await this.getStoredToken();
    
    if (!token) {
      return false;
    }

    this.client.setToken(token);

    try {
      // Test the token by getting status
      await this.client.getStatus();
      this._isAuthenticated = true;
      return true;
    } catch (err) {
      // Token might be invalid
      this._isAuthenticated = false;
      return false;
    }
  }

  /**
   * Initiate the pairing flow
   */
  async pair(): Promise<boolean> {
    try {
      const response = await this.client.pair(CLIENT_NAME);

      if (response.requiresApproval) {
        // Show notification to user
        const result = await vscode.window.showInformationMessage(
          'The music daemon requires approval for this connection. ' +
          'Please approve the connection request on your system.',
          'OK',
          'Cancel'
        );

        if (result === 'Cancel') {
          return false;
        }

        // The daemon should have approved by now
        // Store the token
        await this.storeToken(response.token);
      } else {
        // Auto-approved (test mode)
        await this.storeToken(response.token);
      }

      vscode.window.showInformationMessage('Successfully connected to music daemon!');
      return true;
    } catch (err) {
      vscode.window.showErrorMessage(
        `Failed to connect to music daemon: ${err}`
      );
      return false;
    }
  }

  /**
   * Authenticate - try stored token first, then pair if needed
   */
  async authenticate(): Promise<boolean> {
    // First, try the stored token
    if (await this.tryStoredToken()) {
      return true;
    }

    // No valid token, need to pair
    return this.pair();
  }

  /**
   * Disconnect and clear authentication
   */
  async disconnect(): Promise<void> {
    await this.clearToken();
    this._isAuthenticated = false;
  }
}
