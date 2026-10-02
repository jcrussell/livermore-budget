package structure

// The scopes the mapping rules write on every fact, one per printed schedule.
// Each is spelled here once; internal/project and internal/check name them.
const (
	ScopeAllFundsGross              = "all-funds-gross"
	ScopeRevenueByFund              = "revenue-by-fund"
	ScopeTransfersByFund            = "transfers-by-fund"
	ScopeCIPFundingSources          = "cip-funding-sources"
	ScopeExpenditureByDepartment    = "expenditure-by-department"
	ScopeGeneralFundByCategory      = "general-fund-by-category"
	ScopeExpenditureByFund          = "expenditure-by-fund"
	ScopeDepartmentwideExpenditures = "departmentwide-expenditures"
	ScopeDepartmentFundingSources   = "department-funding-sources"
	ScopeACFRGeneralFundSummary     = "acfr-general-fund-summary"
	ScopeACFRChangesInFundBalances  = "acfr-changes-in-fund-balances"
	ScopeACFRFundBalances           = "acfr-fund-balances"
	ScopeDebtServiceByIssue         = "debt-service-by-issue"
)

// The two tiers a cut's department axis may be read at: pp.167-170's
// divisions, or pp.85-125's departments above them.
const (
	TierDivision   = "division"
	TierDepartment = "department"
)
