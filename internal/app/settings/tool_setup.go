package settings

type ToolSetup struct {
	FromSettings, ContextsOnly, RepositoryOnly bool
	Tool                                       string
	Path, Err                                  string
	RefreshRequired                            bool
	Names                                      map[int]string
	NameIndex                                  int
	Picker                                     *Form
}
