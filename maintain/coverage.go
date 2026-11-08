package maintain

import "github.com/spf13/afero"

// CoverageFloor is the coverage a repository has to keep once it has reached
// it. A run that takes it below fails; one that finds it below already logs.
const CoverageFloor = 80

// RecordCoverage writes pct into the repository's own definition, creating it
// when there is none.
//
// Only the one field is touched: a definition already holding a description
// and topics keeps them, and a repository that has never had one gets a file
// saying the only thing anybody has measured about it so far.
func RecordCoverage(fs afero.Fs, pct float64) error {
	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	def.Coverage = pct

	return SaveDefinition(fs, def)
}
