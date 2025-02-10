/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package benchmark

import "time"

//go:generate protoc --go_out=paths=source_relative:. result.proto

// MaxDuration is an arbitrary duration that should be larger than any simi benchmark-measured duration.
const MaxDuration = 9 * time.Hour
