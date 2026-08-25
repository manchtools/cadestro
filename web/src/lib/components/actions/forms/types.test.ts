import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import {
	RepositoryParamsSchema,
	ZypperRepositorySchema,
	ZypperRepositoryType
} from '$contract/cadestro/v1/actions_pb';
import {
	defaultRepositoryForm,
	repositoryFormToProto,
	repositoryProtoToForm
} from './types';

describe('Zypper repository type mapping', () => {
	it('round-trips every type and preserves the unspecified default', () => {
		const cases = [
			['', ZypperRepositoryType.UNSPECIFIED],
			['rpm-md', ZypperRepositoryType.RPM_MD],
			['yast2', ZypperRepositoryType.YAST2],
			['plaindir', ZypperRepositoryType.PLAINDIR]
		] as const;

		for (const [value, enumValue] of cases) {
			const form = defaultRepositoryForm();
			form.zypper.disabled = false;
			form.zypper.type = value;
			const proto = repositoryFormToProto(form);
			expect(proto.zypper?.type).toBe(enumValue);
			expect(repositoryProtoToForm(proto).zypper.type).toBe(value);
		}
	});

	it('maps an unknown wire value to the unspecified form state', () => {
		const proto = create(RepositoryParamsSchema, {
			zypper: create(ZypperRepositorySchema, {
				type: 99 as ZypperRepositoryType
			})
		});
		expect(repositoryProtoToForm(proto).zypper.type).toBe('');
	});
});
