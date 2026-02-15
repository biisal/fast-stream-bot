package main

import "flag"

type AppFlags struct {
	InitDB bool
}

func perseFlags() AppFlags {
	var initDB = flag.Bool("init-db", false, "Create initial tables(eg. users) in the database")
	flag.Parse()
	return AppFlags{InitDB: *initDB}
}
