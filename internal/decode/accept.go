// Copyright (c) 2013, Ryan Rogers
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
//  1. Redistributions of source code must retain the above copyright notice, this
//     list of conditions and the following disclaimer.
//  2. Redistributions in binary form must reproduce the above copyright notice,
//     this list of conditions and the following disclaimer in the documentation
//     and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
package decode

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var (
	errInvalidTypeSubtype = "accept: Invalid type '%s'."
)

// decodeType will define the type of decoder to use based on the provided Accept header. See the "bestFitDecodeType"
// function for usage.
type decodeType int

const (
	// decodeTypeUnknown is returned when the Accept header does not match any of the provided types.
	decodeTypeUnknown decodeType = iota

	// decodeTypeJSON will decode the request body as JSON.
	decodeTypeJSON
)

// accept represents a parsed accept(-Charset|-Encoding|-Language) header.
type accept struct {
	typ, subtype  string
	qualityFactor float64
	extensions    map[string]string
}

// AcceptSlice is a slice of Accept.
type AcceptSlice []accept

// Len implements the Len() method of the Sort interface.
func (a AcceptSlice) Len() int {
	return len(a)
}

// Less implements the Less() method of the Sort interface.  Elements are
// sorted in order of decreasing preference.
func (a AcceptSlice) Less(i, j int) bool {
	// Higher qvalues come first.
	if a[i].qualityFactor > a[j].qualityFactor {
		return true
	} else if a[i].qualityFactor < a[j].qualityFactor {
		return false
	}

	// Specific types come before wildcard types.
	if a[i].typ != "*" && a[j].typ == "*" {
		return true
	} else if a[i].typ == "*" && a[j].typ != "*" {
		return false
	}

	// Specific subtypes come before wildcard subtypes.
	if a[i].subtype != "*" && a[j].subtype == "*" {
		return true
	} else if a[i].subtype == "*" && a[j].subtype != "*" {
		return false
	}

	// A lot of extensions comes before not a lot of extensions.
	if len(a[i].extensions) > len(a[j].extensions) {
		return true
	}

	return false
}

// Swap implements the Swap() method of the Sort interface.
func (a AcceptSlice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// parseMediaRange parses the provided media range, and on success returns the
// parsed range params and type/subtype pair.
func parseMediaRange(mediaRange string) (rangeParams, typeSubtype []string, err error) {
	rangeParams = strings.Split(mediaRange, ";")
	typeSubtype = strings.Split(rangeParams[0], "/")

	// typeSubtype should have a length of exactly two.
	if len(typeSubtype) > 2 {
		err = fmt.Errorf(errInvalidTypeSubtype, rangeParams[0])
		return
	} else {
		typeSubtype = append(typeSubtype, "*")
	}

	// Sanitize typeSubtype.
	typeSubtype[0] = strings.TrimSpace(typeSubtype[0])
	typeSubtype[1] = strings.TrimSpace(typeSubtype[1])
	if typeSubtype[0] == "" {
		typeSubtype[0] = "*"
	}
	if typeSubtype[1] == "" {
		typeSubtype[1] = "*"
	}

	return
}

// parseAcceptHeader parses a HTTP Accept(-Charset|-Encoding|-Language) header and returns
// AcceptSlice, sorted in decreasing order of preference.  If the header lists
// multiple types that have the same level of preference (same specificity of
// type and subtype, same qvalue, and same number of extensions), the type
// that was listed in the header first comes first in the returned value.
//
// See http://www.w3.org/Protocols/rfc2616/rfc2616-sec14 for more information.
func parseAcceptHeader(header string) AcceptSlice {
	mediaRanges := strings.Split(header, ",")
	accepted := make(AcceptSlice, 0, len(mediaRanges))

	for _, mediaRange := range mediaRanges {
		rangeParams, typeSubtype, err := parseMediaRange(mediaRange)
		if err != nil {
			continue
		}

		accept := accept{
			typ:           typeSubtype[0],
			subtype:       typeSubtype[1],
			qualityFactor: 1.0,
			extensions:    make(map[string]string),
		}

		// If there is only one rangeParams, we can stop here.
		if len(rangeParams) == 1 {
			accepted = append(accepted, accept)
			continue
		}

		// Validate the rangeParams.
		validParams := true
		for _, v := range rangeParams[1:] {
			nameVal := strings.SplitN(v, "=", 2)
			if len(nameVal) != 2 {
				validParams = false
				break
			}
			nameVal[1] = strings.TrimSpace(nameVal[1])
			if name := strings.TrimSpace(nameVal[0]); name == "q" {
				qval, err := strconv.ParseFloat(nameVal[1], 64)
				if err != nil || qval < 0 {
					validParams = false
					break
				}
				if qval > 1.0 {
					qval = 1.0
				}
				accept.qualityFactor = qval
			} else {
				accept.extensions[name] = nameVal[1]
			}
		}

		if validParams {
			accepted = append(accepted, accept)
		}
	}

	sort.Sort(accepted)
	return accepted
}

// bestFitDecodeType will parse the provided Accept(-Charset|-Encoding|-Language) header and return the header that
// best fits the decoding algorithm. If the "Accept" header is not set, then this method will return a decodeTypeJSON.
// If the "Accept" header is set, but no match is found, then this method will return a decodeTypeUnkown.
//
// See the "acceptSlice.Less" method for more informaiton on how the "best fit" is determined.
func bestFitDecodeType(header string) decodeType {
	accepted := parseAcceptHeader(header)

	// If the header is empty, we default to JSON.
	if len(accepted) == 0 {
		return decodeTypeJSON
	}

	for _, accept := range accepted {
		// If the type is "*" and the subtype is "*", then we default to JSON.
		if accept.typ == "*" && accept.subtype == "*" {
			return decodeTypeJSON
		}

		// If the type is "application" and the subtype is "json" or "*", then we use JSON.
		if accept.typ == "application" && (accept.subtype == "json" || accept.subtype == "*") {
			return decodeTypeJSON
		}
	}

	return decodeTypeUnknown
}
