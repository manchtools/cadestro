// Connect-RPC mocks for the showcase harness. Each function takes the
// Playwright Page and registers route handlers that return JSON-encoded
// Connect-RPC unary responses. Connect-Web defaults to JSON for non-Node
// targets, so we don't need a binary protobuf encoder here.
//
// IMPORTANT: page.route is LIFO — the LAST registered handler that
// matches the URL wins. Register the broad catch-all FIRST, then the
// specific RPC-method handlers, so specific overrides catch-all.

import type { Page, Route } from '@playwright/test';
import {
	ALL_PERMISSIONS,
	actionsAsSearchResults,
	actionSetsAsSearchResults,
	auditEventsAsSearchResults,
	compliancePoliciesAsSearchResults,
	definitionsAsSearchResults,
	deviceGroupsAsSearchResults,
	devicesAsSearchResults,
	getActionResponse,
	getActionSetResponse,
	getCompliancePolicyResponse,
	getDefinitionResponse,
	getDeviceByIdResponse,
	getDeviceCompliancePolicyStatusResponse,
	getDeviceGroupResponse,
	getDeviceInventoryResponse,
	getIdentityProviderResponse,
	getRoleResponse,
	getServerSettingsResponse,
	getUserGroupResponse,
	getUserResponse,
	listActionsResponse,
	listActionSetsResponse,
	listActiveTerminalSessionsResponse,
	listAuthMethodsResponse,
	listAvailableActionsResponse,
	listDevicesResponse,
	listIdentityLinksResponse,
	listIdentityProvidersResponse,
	listPermissionsResponse,
	listRolesResponse,
	listTokensResponse,
	listUsersResponse,
	userGroupsAsSearchResults,
	usersAsSearchResults,
} from './dummy';

function unaryJson(payload: unknown): Parameters<Route['fulfill']>[0] {
	return {
		status: 200,
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(payload),
	};
}

// protobuf-es JSON encodes enums by their proto-name (e.g.
// "SEARCH_SCOPE_DEVICES"), not by the integer value. Accept either form
// so the dispatch works whichever way the request was serialized.
function searchResponseFor(scope: number | string | undefined): unknown {
	const s = typeof scope === 'string' ? scope : '';
	if (scope === 1 || s === 'SEARCH_SCOPE_ACTIONS') return actionsAsSearchResults();
	if (scope === 2 || s === 'SEARCH_SCOPE_ACTION_SETS') return actionSetsAsSearchResults();
	if (scope === 3 || s === 'SEARCH_SCOPE_DEFINITIONS') return definitionsAsSearchResults();
	if (scope === 4 || s === 'SEARCH_SCOPE_COMPLIANCE_POLICIES') return compliancePoliciesAsSearchResults();
	if (scope === 5 || s === 'SEARCH_SCOPE_DEVICES') return devicesAsSearchResults();
	if (scope === 6 || s === 'SEARCH_SCOPE_USERS') return usersAsSearchResults();
	if (scope === 7 || s === 'SEARCH_SCOPE_DEVICE_GROUPS') return deviceGroupsAsSearchResults();
	if (scope === 8 || s === 'SEARCH_SCOPE_USER_GROUPS') return userGroupsAsSearchResults();
	if (scope === 9 || s === 'SEARCH_SCOPE_AUDIT_EVENTS') return auditEventsAsSearchResults();
	return { results: [], nextPageToken: '', totalCount: 0 };
}

// Small helper: register a handler that reads the request's `id` and dispatches
// to a fixture builder, so detail routes resolve their entity by id.
function byId(builder: (id: string) => unknown) {
	return async (route: Route) => {
		const id = (route.request().postDataJSON() as { id?: string })?.id ?? '';
		await route.fulfill(unaryJson(builder(id)));
	};
}

export async function mockControlService(page: Page): Promise<void> {
	// Catch-all so any unstubbed RPC returns `{}` rather than the dev
	// server's 404 (which would trigger a toast on every page load).
	await page.route('**/cadestro.v1.ControlService/**', async (route) => {
		await route.fulfill(unaryJson({}));
	});

	// Search dispatches by scope — read the request body and route.
	await page.route('**/cadestro.v1.ControlService/Search', async (route) => {
		const req = route.request();
		let scope: number | string | undefined;
		try {
			const body = req.postDataJSON() as { scope?: number | string };
			scope = body?.scope;
		} catch {
			/* fall through to empty */
		}
		await route.fulfill(unaryJson(searchResponseFor(scope)));
	});

	await page.route('**/cadestro.v1.ControlService/GetDevice', async (route) => {
		const req = route.request();
		const id = (req.postDataJSON() as { id?: string })?.id ?? '';
		await route.fulfill(unaryJson(getDeviceByIdResponse(id)));
	});

	await page.route('**/cadestro.v1.ControlService/GetDeviceInventory', async (route) => {
		await route.fulfill(unaryJson(getDeviceInventoryResponse()));
	});

	await page.route('**/cadestro.v1.ControlService/RefreshDeviceInventory', async (route) => {
		await route.fulfill(unaryJson({}));
	});

	await page.route('**/cadestro.v1.ControlService/ListRoles', async (route) => {
		await route.fulfill(unaryJson(listRolesResponse()));
	});

	await page.route('**/cadestro.v1.ControlService/GetDeviceGroup', async (route) => {
		const id = (route.request().postDataJSON() as { id?: string })?.id ?? '';
		await route.fulfill(unaryJson(getDeviceGroupResponse(id)));
	});

	await page.route('**/cadestro.v1.ControlService/ListDevices', async (route) => {
		await route.fulfill(unaryJson(listDevicesResponse()));
	});

	await page.route('**/cadestro.v1.ControlService/GetDeviceCompliancePolicyStatus', async (route) => {
		await route.fulfill(unaryJson(getDeviceCompliancePolicyStatusResponse()));
	});

	await page.route('**/cadestro.v1.ControlService/ValidateDynamicQuery', async (route) => {
		// Used by the create / edit-query dialogs to preview matching devices.
		await route.fulfill(unaryJson({ valid: true, error: '', matchingDeviceCount: 6 }));
	});

	// GetCurrentUser — the auth store already has the user hydrated from
	// localStorage; this is here for any page that calls it explicitly.
	await page.route('**/cadestro.v1.ControlService/GetCurrentUser', async (route) => {
		await route.fulfill(
			unaryJson({
				user: {
					id: '01J6XYZSHOWCASEADMINUSR01',
					email: 'admin@cadestro.example',
					displayName: 'Sam Reiter',
					givenName: 'Sam',
					familyName: 'Reiter',
					preferredUsername: 'sam.reiter',
					locale: 'en',
					picture: '',
					disabled: false,
					identityLinks: [],
					roleGrants: [
						{
							role: {
								id: '00000000000000000000000001',
								name: 'Administrator',
								// Do not narrow the seeded admin session when a page refreshes it.
								permissions: ALL_PERMISSIONS,
								isSystem: true
							},
							scopeKind: 'ROLE_GRANT_SCOPE_KIND_UNSPECIFIED',
							scopeId: '',
							scopeName: ''
						},
					],
					inheritedRoles: [],
				},
			})
		);
	});
}

// Additional read-RPC handlers covering every route the e2e smoke suite visits.
// Registered AFTER mockControlService so they take precedence over its
// catch-all (page.route is LIFO). Kept separate so the marketing showcase
// (which only needs a handful) is unaffected.
export async function mockControlServiceExtras(page: Page): Promise<void> {
	// Actions / sets / definitions / compliance.
	await page.route('**/cadestro.v1.ControlService/ListActions', async (route) => {
		await route.fulfill(unaryJson(listActionsResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/GetAction', byId(getActionResponse));
	await page.route('**/cadestro.v1.ControlService/ListActionSets', async (route) => {
		await route.fulfill(unaryJson(listActionSetsResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/GetActionSet', byId(getActionSetResponse));
	await page.route('**/cadestro.v1.ControlService/GetDefinition', byId(getDefinitionResponse));
	await page.route('**/cadestro.v1.ControlService/GetCompliancePolicy', byId(getCompliancePolicyResponse));

	// Users / groups / roles / permissions.
	await page.route('**/cadestro.v1.ControlService/ListUsers', async (route) => {
		await route.fulfill(unaryJson(listUsersResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/GetUser', byId(getUserResponse));
	await page.route('**/cadestro.v1.ControlService/GetUserGroup', byId(getUserGroupResponse));
	await page.route('**/cadestro.v1.ControlService/GetRole', byId(getRoleResponse));
	await page.route('**/cadestro.v1.ControlService/ListPermissions', async (route) => {
		await route.fulfill(unaryJson(listPermissionsResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/ListUserGroupsForUser', async (route) => {
		await route.fulfill(unaryJson({ groups: [] }));
	});
	await page.route('**/cadestro.v1.ControlService/ListDeviceGroups', async (route) => {
		await route.fulfill(unaryJson(deviceGroupsList()));
	});

	// Tokens.
	await page.route('**/cadestro.v1.ControlService/ListTokens', async (route) => {
		await route.fulfill(unaryJson(listTokensResponse()));
	});

	// Identity providers / links / auth methods.
	await page.route('**/cadestro.v1.ControlService/ListIdentityProviders', async (route) => {
		await route.fulfill(unaryJson(listIdentityProvidersResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/GetIdentityProvider', byId(getIdentityProviderResponse));
	await page.route('**/cadestro.v1.ControlService/ListIdentityLinks', async (route) => {
		await route.fulfill(unaryJson(listIdentityLinksResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/ListAuthMethods', async (route) => {
		await route.fulfill(unaryJson(listAuthMethodsResponse()));
	});

	// Settings, terminal sessions, and available actions.
	await page.route('**/cadestro.v1.ControlService/GetServerSettings', async (route) => {
		await route.fulfill(unaryJson(getServerSettingsResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/ListActiveTerminalSessions', async (route) => {
		await route.fulfill(unaryJson(listActiveTerminalSessionsResponse()));
	});
	await page.route('**/cadestro.v1.ControlService/ListAvailableActions', async (route) => {
		await route.fulfill(unaryJson(listAvailableActionsResponse()));
	});
}

// ListDeviceGroups returns the DeviceGroup list shape ({ groups }); derive it
// from the same detail fixtures so the list and detail views agree.
function deviceGroupsList(): unknown {
	const ids = ['01J6XYZSHOWCASEDEVGRP0001', '01J6XYZSHOWCASEDEVGRP0002', '01J6XYZSHOWCASEDEVGRP0003'];
	return {
		groups: ids.map((id) => (getDeviceGroupResponse(id) as { group: unknown }).group),
		nextPageToken: '',
		totalCount: ids.length,
	};
}
