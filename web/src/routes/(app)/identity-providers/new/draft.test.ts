import { describe, expect, it } from 'vitest';
import { draftErrors } from './draft';
const draft = { name: ' Provider ', slug: ' Team123 ', clientId: ' client ', issuerUrl: ' https://issuer.example ', scopes: 'openid' };
describe('provider connection validation', () => {
 it('accepts schema-valid alphanumeric slugs and trimmed connection fields', () => {
  expect(draftErrors(draft)).toEqual({});
 });
 it('rejects unsupported slug punctuation and malformed HTTPS URLs', () => {
  expect(draftErrors({ ...draft, slug: 'team-one', issuerUrl: 'https://' })).toMatchObject({ slug: expect.any(String), issuerUrl: expect.any(String) });
  expect(draftErrors({ ...draft, slug: 'x'.repeat(65) }).slug).toBeTruthy();
 });
});
