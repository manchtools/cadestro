import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const actionBasicSchema = z.object({
	name: z.string().min(1, m.validation_name_required()),
	description: z.string().optional().default(''),
	timeoutSeconds: z.number().int().min(1, 'Timeout must be at least 1 second').max(3600)
});

export const shellParamsSchema = z.object({
	script: z.string().min(1, 'Script is required'),
	interpreter: z.string().min(1, 'Interpreter is required'),
	runAsRoot: z.boolean()
});

export const complianceShellParamsSchema = z.object({
	detectionScript: z.string().min(1, 'Detection script is required'),
	interpreter: z.string().min(1, 'Interpreter is required'),
	runAsRoot: z.boolean()
});

export const packageParamsSchema = z
	.object({
		name: z.string(),
		version: z.string(),
		allowDowngrade: z.boolean(),
		aptName: z.string(),
		dnfName: z.string(),
		pacmanName: z.string(),
		zypperName: z.string()
	})
	.refine(
		(data) =>
			data.name.trim() ||
			data.aptName.trim() ||
			data.dnfName.trim() ||
			data.pacmanName.trim() ||
			data.zypperName.trim(),
		{ message: 'At least one package name is required', path: ['name'] }
	);

export const serviceParamsSchema = z.object({
	unitName: z.string().min(1, 'Unit name is required'),
	desiredState: z.string().min(1),
	enable: z.boolean()
});

export const fileParamsSchema = z.object({
	path: z.string().min(1, 'File path is required'),
	content: z.string(),
	owner: z.string(),
	group: z.string(),
	mode: z.string()
});

export const appParamsSchema = z.object({
	url: z.string().min(1, 'Download URL is required').url('Must be a valid URL'),
	checksumSha256: z.string(),
	installPath: z.string()
});

export const flatpakParamsSchema = z.object({
	appId: z.string().min(1, 'App ID is required'),
	remote: z.string(),
	systemWide: z.boolean(),
	pin: z.boolean()
});

export const updateParamsSchema = z.object({
	securityOnly: z.boolean(),
	autoremove: z.boolean(),
	rebootIfRequired: z.boolean()
});

export const repositoryParamsSchema = z
	.object({
		name: z.string().min(1, 'Repository name is required'),
		apt: z.object({ disabled: z.boolean() }).passthrough(),
		dnf: z.object({ disabled: z.boolean() }).passthrough(),
		pacman: z.object({ disabled: z.boolean() }).passthrough(),
		zypper: z.object({ disabled: z.boolean() }).passthrough()
	})
	.refine(
		(data) => !(data.apt.disabled && data.dnf.disabled && data.pacman.disabled && data.zypper.disabled),
		{
			message: 'At least one package manager must be enabled',
			path: ['name']
		}
	);

export const directoryParamsSchema = z.object({
	path: z.string().min(1, 'Directory path is required'),
	owner: z.string(),
	group: z.string(),
	mode: z.string(),
	recursive: z.boolean()
});

export const userParamsSchema = z.object({
	username: z.string().min(1, 'Username is required'),
	uid: z.number().int().min(0).max(65534).optional(),
	gid: z.number().int().min(0).max(65534).optional(),
	homeDir: z.string(),
	shell: z.string(),
	comment: z.string(),
	systemUser: z.boolean(),
	createHome: z.boolean(),
	disabled: z.boolean(),
	primaryGroup: z.string(),
	hidden: z.boolean()
});

export const groupParamsSchema = z
	.object({
		name: z.string().min(1, 'Group name is required').max(32),
		members: z.array(z.string().min(1).max(32)),
		gid: z.number().int().min(0).max(65534).optional(),
		systemGroup: z.boolean()
	})
	.refine(
		(data) => data.members.map((u) => u.trim()).filter(Boolean).length > 0,
		{ message: 'At least one member is required', path: ['members'] }
	);

export const sshParamsSchema = z
	.object({
		users: z.array(z.string()).min(1, 'At least one user is required'),
		allowPubkey: z.boolean(),
		allowPassword: z.boolean()
	})
	.refine(
		(data) => data.users.map((u) => u.trim()).filter(Boolean).length > 0,
		{ message: 'At least one user is required', path: ['users'] }
	);

export const sshdParamsSchema = z.object({
	directives: z
		.array(
			z.object({
				key: z.string().min(1, 'Directive key is required'),
				value: z.string().min(1, 'Directive value is required')
			})
		)
		.min(1, 'At least one directive is required')
});

export const adminPolicyParamsSchema = z
	.object({
		accessLevel: z.string().min(1, 'Access level is required'),
		users: z.array(z.string()).min(1, 'At least one user is required'),
		customConfig: z.string()
	})
	.refine(
		(data) => data.users.map((u) => u.trim()).filter(Boolean).length > 0,
		{ message: 'At least one user is required', path: ['users'] }
	)
	.refine(
		(data) => data.accessLevel !== 'CUSTOM' || (data.customConfig?.trim() ?? '').length > 0,
		{ message: 'Custom configuration is required', path: ['customConfig'] }
	);

export const lpsParamsSchema = z.object({
	usernames: z.array(z.string().min(1).max(32)).min(1, 'At least one username is required'),
	passwordLength: z.number().int().min(8).max(128),
	complexity: z.string().min(1, 'Complexity is required'),
	rotationIntervalDays: z.number().int().min(1).max(365),
	gracePeriodHours: z.number().int().min(0).max(8760)
});

export const encryptionParamsSchema = z
	.object({
		presharedKey: z.string(),
		presharedKeyConfigured: z.boolean(),
		rotationIntervalDays: z.number().int().min(1).max(365),
		minWords: z.number().int().min(3).max(10),
		deviceBoundKeyType: z.string().min(1),
		userPassphraseMinLength: z.number().int().min(0).max(128),
		userPassphraseComplexity: z.string()
	})
	.refine((data) => data.presharedKey.length > 0 || data.presharedKeyConfigured, {
		message: 'Pre-shared key is required',
		path: ['presharedKey']
	});

export const wifiParamsSchema = z
	.object({
		ssid: z.string().min(1, 'SSID is required').max(255),
		authType: z.string().min(1, 'Auth type is required'),
		psk: z.string().max(63).optional(),
		pskConfigured: z.boolean(),
		caCert: z.string().optional(),
		clientCert: z.string().optional(),
		clientKey: z.string().optional(),
		clientKeyConfigured: z.boolean(),
		identity: z.string().max(254).optional(),
		autoConnect: z.boolean(),
		hidden: z.boolean(),
		priority: z.number().int().min(-1).max(999)
	})
	.refine(
		(data) => data.authType !== 'PSK' || (data.psk?.length ?? 0) >= 8 || data.pskConfigured,
		{ message: 'PSK must be at least 8 characters', path: ['psk'] }
	)
	.refine(
		(data) => data.authType !== 'EAP_TLS' || (data.identity?.trim() ?? '').length > 0,
		{ message: 'Identity is required for EAP-TLS', path: ['identity'] }
	)
	.refine(
		(data) => data.authType !== 'EAP_TLS' || (data.clientKey?.length ?? 0) > 0 || data.clientKeyConfigured,
		{ message: 'Client private key is required for EAP-TLS', path: ['clientKey'] }
	);

const httpsUrl = z.string().refine(
	(val) => !val || val.startsWith('https://'),
	{ message: 'URL must use HTTPS' }
);

export const agentUpdateParamsSchema = z
	.object({
		amd64BinaryUrl: httpsUrl,
		amd64ChecksumUrl: httpsUrl,
		arm64BinaryUrl: httpsUrl,
		arm64ChecksumUrl: httpsUrl
	})
	.refine(
		(data) => data.amd64BinaryUrl.trim() || data.arm64BinaryUrl.trim(),
		{ message: 'At least one architecture must be configured', path: ['amd64BinaryUrl'] }
	)
	.refine(
		(data) => !data.amd64BinaryUrl.trim() || data.amd64ChecksumUrl.trim(),
		{ message: 'Checksum URL is required when binary URL is set', path: ['amd64ChecksumUrl'] }
	)
	.refine(
		(data) => !data.arm64BinaryUrl.trim() || data.arm64ChecksumUrl.trim(),
		{ message: 'Checksum URL is required when binary URL is set', path: ['arm64ChecksumUrl'] }
	);

export const scheduleSchema = z.object({
	cron: z.string(),
	intervalHours: z.number().int().min(1).max(8760),
	runOnAssign: z.boolean(),
	skipIfUnchanged: z.boolean()
});
