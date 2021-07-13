// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package common

type ChunkID uint64

type ChunkIDs []ChunkID

// HashIntersect returns an intersection of two ChunkID arrays
// without any assumptions on the order. It uses a HashMap to mark
// the existence of a item.
func (c ChunkIDs) HashIntersect(other ChunkIDs) ChunkIDs {
	if len(c) == 0 || len(other) == 0 {
		return []ChunkID{}
	}
	intersection := make([]ChunkID, 0, min(len(c), len(other)))
	hash := make(map[ChunkID]struct{})
	for _, item := range c {
		hash[item] = struct{}{}
	}
	for _, item := range other {
		if _, exist := hash[item]; exist {
			intersection = append(intersection, item)
		}
	}
	return intersection
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
