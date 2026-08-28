SDK:

why does the server have a ca.FingerprintFromPEM if the SDK already has a function for that -> CAFingerprintFromPEM -> why not have one cert handling method for every consumer? Certs are not that special to have 3 unique implementations

already inside a couple files i see error handling is all over the place. The SDK does not provide typed errors that can be used for a errors.Is comparison. We need a error package in the SDK where errors are exported so consumers can import them, just like any SDK does

why doesnt GenerateCSRFromKey call GenerateCRS and rather implements the same code again?

the sdk also still contains power-manage references like pmexec inside the pkg package

why are aptWriteEnv and dpkgConfOptions a var and not a const?

Why does Manager.InstalledVersion not return an error if the package can not be found?

Do we really need a Flatpak manager? Could those options not be more general because APT and DNF both have repo management as well?

what is apts.cmdOnce?

why does apt probe for apt/apt-get if detect already does that?

what is hasApt used for?

The install command takes in one InstallOptions but multiple packages. Passing one version to multiple packages will break install -> just have a defined "Package" struct (we already have that) that can be passed with per package versions, or loose the multi package install and let it just install one package

ValidateLocalPackagePath -> dont we have multiple path validation and checking helpers already?

why is the len(packages) == 0 validation done after other validators has run? That is useless at that point. That needs to be moved up.

why does Upgrade have 2 append calls to args when you could just have one?

Why dont we have a seperate "SecurityUpdate" function on the interface. That would make it cleaner to just do security updates instead of calling the upgrades with a config option

bestEffortStep -> do we really need that abstraction?

is ValidateSearchQuery properlly written as that is one path to a shell escape i can see, as from what i can see it looks a little bit "weak"

Search/List -> why do we need a scanner to operate on a already returned full string?

HasUpdates -> why can apt install security only updates but not check if there are any?

Why does ValidateCommandEnv check for isReservedEnvVar if that check would be better of inside validateEnvVars?

what is runStreamingWithStdin used for? Terminal?

what i cappes_buffer used for? i thought go-cmd already has some good buffer handing and good streaming?

in general the exec package looks more like reinventing what go-cmd already offers

resolveAnchorsDir -> if anchorsDirExist cant find anything it returs the first value, that does not make sense, because it clearly does not exist, why is it returned?

why do we need a fsManager for Certificates?

