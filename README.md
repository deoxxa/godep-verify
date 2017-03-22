# godep-verify

This tool will verify that the contents of the `vendor` directory is correct
in a project using godep.

## Usage

```
Usage of ./godep-verify:
  -manifest string
    	Manifest file with dependencies. (default "Godeps/Godeps.json")
  -vendor string
    	Vendor directory holding dependencies. (default "vendor")
  -cache string
    	Temporary directory for checking out sources. (default "/tmp")
  -v	Turn on verbose logging.
```

## Operation

The way the program works is as such:

1. Read the manifest file.
2. Resolve all the packages to their source URLs using the same logic as `go
   get`.
3. Fetch all the dependencies from their sources and check out the correct
   revisions.
4. Walk the `vendor` tree, comparing each file to the same file we just
   checked out from the source.

If there are any differences, they'll be displayed on stdout, and the program
will exit with a non-zero return code. This makes it suitable for use in a CI
environment.

## Known Issues

* godep itself will strip canonical import comments from packages, even when
  using the `vendor` directory. This may be a bug in godep at this point. The
  result is that most runs of this tool will currently fail.
* Currently this tool only supports git. Other version control systems can be
  added in the future if necessary.
