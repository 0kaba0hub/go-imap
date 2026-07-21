package imapclient_test

import (
	"reflect"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestSearch(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	criteria := imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{
			Key:   "Message-Id",
			Value: "<191101702316132@example.com>",
		}},
	}
	data, err := client.Search(&criteria, nil).Wait()
	if err != nil {
		t.Fatalf("Search().Wait() = %v", err)
	}
	seqSet, ok := data.All.(imap.SeqSet)
	if !ok {
		t.Fatalf("SearchData.All = %T, want SeqSet", data.All)
	}
	nums, _ := seqSet.Nums()
	want := []uint32{1}
	if !reflect.DeepEqual(nums, want) {
		t.Errorf("SearchData.All.Nums() = %v, want %v", nums, want)
	}
}

func TestESearch(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	if !client.Caps().Has(imap.CapESearch) {
		t.Skip("server doesn't support ESEARCH")
	}

	criteria := imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{
			Key:   "Message-Id",
			Value: "<191101702316132@example.com>",
		}},
	}
	options := imap.SearchOptions{
		ReturnCount: true,
	}
	data, err := client.Search(&criteria, &options).Wait()
	if err != nil {
		t.Fatalf("Search().Wait() = %v", err)
	}
	if want := uint32(1); data.Count != want {
		t.Errorf("Count = %v, want %v", data.Count, want)
	}
}

func TestESearch_Relevancy(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	if !client.Caps().Has(imap.CapESearch) {
		t.Skip("server doesn't support ESEARCH")
	}

	criteria := imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{
			Key:   "Message-Id",
			Value: "<191101702316132@example.com>",
		}},
	}
	options := imap.SearchOptions{
		ReturnAll:       true,
		ReturnRelevancy: true,
	}
	data, err := client.Search(&criteria, &options).Wait()
	if err != nil {
		t.Fatalf("Search().Wait() = %v", err)
	}
	if len(data.Relevancy) != 1 {
		t.Fatalf("len(Relevancy) = %v, want 1", len(data.Relevancy))
	}
	if data.Relevancy[0] < 1 || data.Relevancy[0] > 100 {
		t.Errorf("Relevancy[0] = %v, want in [1, 100]", data.Relevancy[0])
	}
}

func TestSearch_NoRelevancyWhenNotRequested(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	criteria := imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{
			Key:   "Message-Id",
			Value: "<191101702316132@example.com>",
		}},
	}
	data, err := client.Search(&criteria, nil).Wait()
	if err != nil {
		t.Fatalf("Search().Wait() = %v", err)
	}
	if len(data.Relevancy) != 0 {
		t.Errorf("Relevancy = %v, want empty when not requested", data.Relevancy)
	}
}
