package torrent

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

// Use Python to calculate byte strings for verifying SHA1 hashes:
//
// import hashlib; input = "testing";
// print '[]byte{' + ', '.join([str(ord(q)) for q in hashlib.sha1(input).digest()]) + '}'

func TestMakeHashesEmptyFile(t *testing.T) {
	test_file := ""
	reader := strings.NewReader(test_file)
	hashes, bytesRead, err := makeHashes(reader, 0)
	assert.Nil(t, err)
	assert.Equal(t, bytesRead, int64(0), "should be 0")
	assert.Empty(t, hashes)
}

func TestMakeHashesOneChunk(t *testing.T) {
	test_file := "testing"
	reader := strings.NewReader(test_file)
	hashes, bytesRead, err := makeHashes(reader, int64(len(test_file)))
	assert.Nil(t, err)
	assert.Equal(t, bytesRead, int64(len(test_file)), "should match the length of the file")
	assert.Len(t, hashes, 1, "should have one chunk")

	bytes := []byte{220, 114, 74, 241, 143, 189, 212, 229, 145, 137, 245, 254, 118, 138, 95,
		131, 17, 82, 112, 80}
	assert.Equal(t, hashes[0], bytes)
}

func TestMakeHashesTwoChunks(t *testing.T) {
	// Two chunks has to be PIECE_LENGTH*1.5 in length so we don't burble up into three, etc.
	test_file := "testing"
	test_file = strings.Repeat(test_file, int((float64(PIECE_LENGTH/int64(len(test_file))))*1.5))

	reader := strings.NewReader(test_file)
	hashes, bytesRead, err := makeHashes(reader, int64(len(test_file)))
	assert.Nil(t, err)
	assert.Equal(t, bytesRead, int64(len(test_file)), "should match the length of the file")
	assert.Len(t, hashes, 2, "should have one chunk")

	bytes := []byte{247, 218, 76, 179, 188, 125, 55, 53, 207, 46, 38, 44, 239, 5, 213,
		222, 176, 165, 62, 232}
	assert.Equal(t, hashes[0], bytes)

	bytes = []byte{34, 54, 176, 174, 156, 227, 107, 165, 9, 94, 63, 190, 216, 216, 205,
		175, 183, 114, 183, 201}
	assert.Equal(t, hashes[1], bytes)
}
