package main

import "flag"

type AppFlags struct {
	InitDB bool
	Ref    bool
}

func perseFlags() AppFlags {
	var initDB = flag.Bool("init-db", false, "Create initial tables(eg. users) in the database")
	var ref = flag.Bool("ref", false, "Enable strict credit checking")
	flag.Parse()
	return AppFlags{InitDB: *initDB, Ref: *ref}
}
