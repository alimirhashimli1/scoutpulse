package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/scoutpulse/football-svc/internal/domain"
)

func fee(v int64) *int64 { return &v }

func TestFoldIgnoresAccentsCaseAndPunctuation(t *testing.T) {
	assert.Equal(t, "atletico de madrid", fold("Atlético de Madrid"))
	assert.Equal(t, "martin odegaard", fold("Martin Ødegaard"))
	assert.Equal(t, "vinicius junior", fold("Vinícius  Júnior"))
	assert.Equal(t, "n golo kante", fold("N'Golo Kanté"))
	assert.Equal(t, "fenerbahce", fold("Fenerbahçe"))
}

func TestClubKeyDropsLegalForm(t *testing.T) {
	assert.Equal(t, clubKey("Arsenal"), clubKey("Arsenal FC"))
	assert.Equal(t, clubKey("Atlético Madrid"), clubKey("Atlético de Madrid"))
	assert.Equal(t, "milan", clubKey("AC Milan"))
	// A name made only of affixes keeps them rather than folding to nothing.
	assert.Equal(t, "ac", clubKey("AC"))
}

func TestSeasonLabel(t *testing.T) {
	assert.Equal(t, "2025/26", seasonLabel(2025))
	assert.Equal(t, "1999/00", seasonLabel(1999))
}

func TestCompetitionsForBringsCupsOnlyWithTheirLeague(t *testing.T) {
	got := competitionsFor([]string{"GB1"})
	assert.Contains(t, got, "GB1")
	assert.Contains(t, got, "FAC")
	assert.Contains(t, got, "GBCS")
	assert.Contains(t, got, "CL")
	assert.NotContains(t, got, "CDR")
	assert.NotContains(t, got, "ES1")
}

func types(moves []Move) []domain.TransferType {
	out := make([]domain.TransferType, len(moves))
	for i, m := range moves {
		out[i] = m.Type
	}
	return out
}

func TestClassifyFindsLoansAndTheirReturns(t *testing.T) {
	// João Félix, 2023-2025, as the source records it.
	moves := Classify([]RawTransfer{
		{Date: "2024-08-21", FromID: "13", ToID: "631", Fee: fee(52_000_000)},
		{Date: "2023-01-11", FromID: "13", ToID: "631", Fee: fee(0)},
		{Date: "2023-06-30", FromID: "631", ToID: "13", Fee: fee(0)},
		{Date: "2023-09-01", FromID: "13", ToID: "131", Fee: fee(0)},
		{Date: "2024-06-30", FromID: "131", ToID: "13", Fee: fee(0)},
		{Date: "2025-02-03", FromID: "631", ToID: "5", Fee: fee(0)},
		{Date: "2025-06-30", FromID: "5", ToID: "631", Fee: fee(0)},
		{Date: "2025-07-29", FromID: "631", ToID: "18544", Fee: fee(35_000_000)},
	})

	assert.Equal(t, []domain.TransferType{
		domain.TransferLoan, domain.TransferLoanReturn,
		domain.TransferLoan, domain.TransferLoanReturn,
		domain.TransferPermanent,
		domain.TransferLoan, domain.TransferLoanReturn,
		domain.TransferPermanent,
	}, types(moves))
	assert.Equal(t, int64(5_200_000_000), *moves[4].FeeMinor, "fees are stored in cents")
	assert.Equal(t, int64(0), *moves[1].FeeMinor)
}

func TestClassifyLeavesAFreeMoveWithoutAReturnAsFree(t *testing.T) {
	moves := Classify([]RawTransfer{
		{Date: "2022-07-01", FromID: "1", ToID: "2", Fee: fee(0)},
		// Moves on from club 2 to a third club: not a return.
		{Date: "2023-06-30", FromID: "2", ToID: "3", Fee: fee(10_000_000)},
		{Date: "2024-06-30", FromID: "3", ToID: "2", Fee: fee(0)},
	})
	assert.Equal(t, domain.TransferFree, moves[0].Type)
	assert.Equal(t, domain.TransferPermanent, moves[1].Type)
	assert.Equal(t, domain.TransferFree, moves[2].Type)
}

func TestClassifyAReversalOutsideTheWindowIsNotALoan(t *testing.T) {
	moves := Classify([]RawTransfer{
		{Date: "2019-07-01", FromID: "1", ToID: "2", Fee: fee(0)},
		{Date: "2023-07-01", FromID: "2", ToID: "1", Fee: fee(0)},
	})
	assert.Equal(t, domain.TransferFree, moves[0].Type)
	assert.Equal(t, domain.TransferFree, moves[1].Type)
}

func TestClassifyReadsTheSourcesPlaceholders(t *testing.T) {
	moves := Classify([]RawTransfer{
		{Date: "2015-07-01", FromID: "9", FromName: "Benfica U19", ToID: "1"},
		{Date: "2016-07-01", FromID: "1", ToID: "2", Fee: nil},
		{Date: "2020-07-01", FromID: "2", ToID: "515", ToName: "Without Club"},
		{Date: "2021-01-01", FromID: "515", FromName: "Without Club", ToID: "3", Fee: fee(0)},
		{Date: "2024-07-01", FromID: "3", ToID: "123", ToName: "Retired"},
	})
	assert.Equal(t, []domain.TransferType{
		domain.TransferYouthPromotion,
		domain.TransferPermanent,
		domain.TransferReleased,
		domain.TransferFree,
		domain.TransferRetired,
	}, types(moves))
	assert.Nil(t, moves[1].FeeMinor, "no fee in the source is an undisclosed fee, not a free one")
}
