package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemHealthProbeWritesAndCleansTemporaryFile(t *testing.T){
	root:=t.TempDir();store:=NewFilesystem(root)
	if err:=store.Health();err!=nil{t.Fatal(err)}
	if _,err:=os.Stat(filepath.Join(root,".shaderflow-health"));!os.IsNotExist(err){t.Fatalf("health artifact remained: %v",err)}
}
