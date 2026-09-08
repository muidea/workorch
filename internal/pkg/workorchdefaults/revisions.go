package workorchdefaults

// KnownBuiltinContentRevision identifies reviewed, immutable seed content. It
// never authorizes a write: each resource owner checks its current object and
// the exact requested target before an explicit upgrade. Unknown content is
// deliberately not treated as a builtin baseline.
func KnownBuiltinContentRevision(kind, id, contentHash string) string {
	for _, value := range reviewedBuiltinRevisions {
		if value.kind == kind && value.id == id && value.contentHash == contentHash {
			return value.revision
		}
	}
	return ""
}

// Fingerprints were derived from the named Git revisions using each owner's
// canonical content hashing. Historical runtime/configuration data is never
// decoded or rewritten by this table.
var reviewedBuiltinRevisions = [...]struct{ kind, id, contentHash, revision string }{
	{"soul", "soul-software-dev-execution", "soul-content-sha256:ec368afcab7a2e42755deb5554115a81bf4d95b769506d25314817d843a9122f", "983be412"},
	{"workspace_template", "software-dev", "workspace-template-content-sha256:7dfa8bc304eeb9ce9f019cf754cea499906b63bad055349b3951ec43b126e554", "0317a3f6"},
	{"soul", "soul-software-dev-execution", "soul-content-sha256:a3dbed514ff31b70b025a1d262bf68cb2c7735e66c05c4ad8124bdabde0c1dac", "0317a3f6"},
	{"soul", "soul-software-dev-validation", "soul-content-sha256:1c058c88072a4f71e5694dd4b8d14561a58cb078213af4c31d79d9d886249bfb", "0317a3f6"},
	{"soul", "soul-software-dev-execution", "soul-content-sha256:396bb1ee6c468959a0681b5d4c91927d61207bf1c9348cebeae01209dc920505", "0be3a5f3"},
	{"soul", "soul-software-dev-execution", "soul-content-sha256:93c6b3419a89d09070d9320a97b7bfb265e99befdc73fda982b1aae5949237b7", "72bcd3ec"},
	{"soul", "soul-generic-governed-planning", "soul-content-sha256:f5a926d08dfa05ca0a51cb7dfbb72d5b28b374bd51b2c0ff535904f8700b7ff0", "fe73385d"},
	{"soul", "soul-generic-governed-execution", "soul-content-sha256:b7948cd1cac04982bff7901a45994156fd3d02923c6da218281379d10b540b2d", "fe73385d"},
	{"soul", "soul-generic-governed-decision", "soul-content-sha256:53d7512429a991e1dba462fca8f298463440bcf173fd562024aa90fbdf6539be", "fe73385d"},
	{"soul", "soul-generic-governed-approval", "soul-content-sha256:2b763000f3a06889643b0aa8e882e1f1df0089081e0990b44acd0a231e6fb5b7", "fe73385d"},
	{"soul", "soul-generic-governed-validation", "soul-content-sha256:bd809fe224dd766487d2fd1f9218ceda5f05f9dad028404379f538f09b1849d1", "fe73385d"},
	{"soul", "soul-generic-governed-artifact", "soul-content-sha256:e6990d8bcb541b2396403d2a01c30f3560171b710c3ca2d43c229f6027ba5751", "fe73385d"},
	{"soul", "soul-weather-query-planning", "soul-content-sha256:fde86b711db60c288041d1d1c5521cad0de258211d9925d9549f892d1124c278", "fe73385d"},
	{"soul", "soul-weather-query-execution", "soul-content-sha256:9d5312e7e6134c0a88f71807f1ace8015bd1a8b26b858fed2c82ef514284b429", "fe73385d"},
	{"soul", "soul-weather-query-decision", "soul-content-sha256:c04bcf4fd1a4bc761623a2a6563fe5aba83aef794eae62374c95c7630528364f", "fe73385d"},
	{"soul", "soul-weather-query-approval", "soul-content-sha256:3b3ea498e857774507d659c1969423423cf36d02c57cd7521898fe245b59d249", "fe73385d"},
	{"soul", "soul-weather-query-validation", "soul-content-sha256:37bdab05b568c7c362dc26d74b65543757c78854386c5d5da5ab04e813594d35", "fe73385d"},
	{"soul", "soul-weather-query-artifact", "soul-content-sha256:293f00dcb5d8814112a75a73007f488c364c76f05e08e8b632a214282820aef0", "fe73385d"},
	{"soul", "soul-software-dev-planning", "soul-content-sha256:b298e9f0691260e9aec2563db4cf0de2133645545054a1865a821e86d605bb31", "fe73385d"},
	{"soul", "soul-software-dev-execution", "soul-content-sha256:902992d62f52413de538e6a8d9624cce06537c8cbcfe0956482b09f80bc4f225", "fe73385d"},
	{"soul", "soul-software-dev-decision", "soul-content-sha256:d2d6b2c5432e6157803099cde3ead1c9d0ba4318627e51d03929743b79a5fb8d", "fe73385d"},
	{"soul", "soul-software-dev-approval", "soul-content-sha256:f325544572a27eca8aa0f6d8fc01eaa0e037d95c7ffa8952bf8475aaffc3014e", "fe73385d"},
	{"soul", "soul-software-dev-validation", "soul-content-sha256:99c738f2cbaf8b0bccd6b56bede4f5c081b79de72584d49b326afc1627b02973", "fe73385d"},
	{"soul", "soul-software-dev-artifact", "soul-content-sha256:4c731461840adbb12a5ebe516ee5f1c332a9b2cf9845c1e14e05e8fd9fe03734", "fe73385d"},
	{"soul", "soul-code-review-planning", "soul-content-sha256:a03aeaada5efb923dbe7d220dd4a82ef1ac5e09fb1f053b7ecbfad4abe99c434", "fe73385d"},
	{"soul", "soul-code-review-execution", "soul-content-sha256:efbdb9cf3ff289973c548e40624e8f56824d7e78b4cbb395593f4ee4b51d0fac", "fe73385d"},
	{"soul", "soul-code-review-decision", "soul-content-sha256:4a4c1d2f490497547b21ebff4251ecd4e062fb8800e2c60bf9bdc47427c7bd53", "fe73385d"},
	{"soul", "soul-code-review-approval", "soul-content-sha256:2eae5d62df0bab50a20f9cbb20b08bd37f5a16a5fb84a8b9a99f3b90db9619e9", "fe73385d"},
	{"soul", "soul-code-review-validation", "soul-content-sha256:8eff1d69d7d8072ed2c2d5db81bd6fccfbcefe6f29f7b7776708ded5a17bc443", "fe73385d"},
	{"soul", "soul-code-review-artifact", "soul-content-sha256:6eb1ea08aaef163499678fe1882afd1b98b0cf9c70e833dd6099f12b63a2fc90", "fe73385d"},
	{"soul", "soul-doc-generator-planning", "soul-content-sha256:da32fa5b2aacef6401a123b6886893a1f4f3f7930e36350b9b6739b0e6ab789a", "fe73385d"},
	{"soul", "soul-doc-generator-execution", "soul-content-sha256:1639202394c8ee9d4ff1ecd97d9a09319908be986fa1c46052a312f2da2437ea", "fe73385d"},
	{"soul", "soul-doc-generator-decision", "soul-content-sha256:721a8773208172ad43107c9c1d94afd1d300aff197058cec4725bac04c1564fa", "fe73385d"},
	{"soul", "soul-doc-generator-approval", "soul-content-sha256:64eb8d1aa900084d60763060a32d1228697d9fc827da84680b9a3a4389fdd414", "fe73385d"},
	{"soul", "soul-doc-generator-validation", "soul-content-sha256:bacb030461029f9da505c681cfc0b1ea3cc825f34b1676fffc0aa7cc9a1bae27", "fe73385d"},
	{"soul", "soul-doc-generator-artifact", "soul-content-sha256:f26cbcd1c0c09c0c8a925582b04ac6b6ab33bc3babc0afd5050b87e486cd4227", "fe73385d"},
	{"soul", "soul-data-analysis-planning", "soul-content-sha256:87b70752fd342d4d5502ed67bff689caf88d286986797c66f847dd7825836211", "fe73385d"},
	{"soul", "soul-data-analysis-execution", "soul-content-sha256:701f2b60730e16be8ea20a12a914a40cab8a55e3945d58b505ca13f1487c421b", "fe73385d"},
	{"soul", "soul-data-analysis-decision", "soul-content-sha256:28fcf06b393766ec8577a61fbfcc0aaa7f11787a8cbcc67257aabfdaf994a68b", "fe73385d"},
	{"soul", "soul-data-analysis-approval", "soul-content-sha256:57a23d91a67ed19b82c563b1b5d495eeeb70bf22f563c5e0b4673c4bcf31bc80", "fe73385d"},
	{"soul", "soul-data-analysis-validation", "soul-content-sha256:e4b07f9eecaad3c45c61c9eb5081ac01e8cbb9e7b28dda0623ce9b5890646e91", "fe73385d"},
	{"soul", "soul-data-analysis-artifact", "soul-content-sha256:c7d5ed3ef70719f91ea38c0d47d725adc10dd54177c980350dd8db5ca8d7fda7", "fe73385d"},
	{"workspace_template", "generic-governed", "workspace-template-content-sha256:34b2fcaaaab4a8bc9e07bf5a999acee420e2405057d13b5005cde6e001ed7019", "fe73385d"},
	{"workspace_template", "weather-query", "workspace-template-content-sha256:c85d56d2159a84f27a61108b5fb2b2992ebc53022d7f535840b04eeb470362b1", "fe73385d"},
	{"workspace_template", "software-dev", "workspace-template-content-sha256:328c0f442fa57ba4f0808948c474e755d1ec72b5743c5e4bf5252491cc2ed9d8", "fe73385d"},
	{"workspace_template", "code-review", "workspace-template-content-sha256:072fa7ec839142271c7e41b2e3bc5a4645c7a170bb75f01ee7bd7176bbaa1930", "fe73385d"},
	{"workspace_template", "doc-generator", "workspace-template-content-sha256:f225868bd8a54b65b3759e8b093bf5740fc6d53a2d3ea783461c2472a348601f", "fe73385d"},
	{"workspace_template", "data-analysis", "workspace-template-content-sha256:b0267c093a0bd46f8bba9bc5330adf40fd1d5ceaf6f924f95fc2c10ee2d320aa", "fe73385d"},
	{"workspace_template", "software-dev", "workspace-template-content-sha256:d8e7d5a2a69e76373023eefae3b788fbe24beed8dd2bdc6a84ca7a9b2feb36f1", "3090726f"},
}
