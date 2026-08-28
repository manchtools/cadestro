import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export * from '../gen/ts/cadestro/v1/actions_pb.js';
export * from '../gen/ts/cadestro/v1/agent_pb.js';
export * from '../gen/ts/cadestro/v1/common_pb.js';
export * from '../gen/ts/cadestro/v1/control_pb.js';
export * from './logger.js';

export function formatTimestamp(timestamp: Timestamp | undefined): string {
	return timestamp ? timestampDate(timestamp).toLocaleString() : 'Never';
}
