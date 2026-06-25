package consoleassets

import "embed"

//go:embed dist/index.html dist/workorch.png dist/assets/*
var Assets embed.FS
