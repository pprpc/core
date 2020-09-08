#!/bin/bash
  
for dir in `ls`;
do
    if [ -d "./${dir}" ];then
        if [ -d "./${dir}/.git" ];then
            echo "Proj ${dir}, git push..."
            cd ${dir}
            #git pull
            #git remote set-url origin --push --add gogs@git.frndtech.com:pprpc/${dir}.git
            git remote set-url origin --push --delete gogs@git.frndtech.com:pprpc/${dir}.git
            git remote set-url origin --add gogs@git.frndtech.com:pprpc/${dir}.git
            git push -f
            cd ..
        fi
    fi
done
