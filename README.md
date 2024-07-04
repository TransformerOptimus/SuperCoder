
<p align="center">
  <a href="https://superagi.com//#gh-light-mode-only">
    <img src="https://superagi.com/wp-content/uploads/2024/07/SuperCoder-dark.png" width="318px" alt="SuperCoder Dark logo" />
  </a>
  <a href="https://superagi.com//#gh-dark-mode-only">
    <img src="https://superagi.com/wp-content/uploads/2024/07/SuperCoder-light.png" width="318px" alt="SuperCoder Light logo" />
  </a>

</p>

<p align="center"><i>Open Source Autonomous Software Development System</i></p>
    

<p align="center">
<a href="https://superagi.com"> <img src="https://superagi.com/wp-content/uploads/2023/08/Website.svg"></a>


<p align="center">
<a href="https://github.com/TransformerOptimus/SuperCoder/fork" target="blank">
<img src="https://img.shields.io/github/forks/TransformerOptimus/SuperCoder?style=for-the-badge" alt="SuperCoder Forks"/>
</a>

<a href="https://github.com/TransformerOptimus/SuperCoder/stargazers" target="blank">
<img src="https://img.shields.io/github/stars/TransformerOptimus/SuperCoder?style=for-the-badge" alt="SuperCoder stars"/>
</a>

</p>

<p align="center"><b>Follow SuperAGI </b></p>

<p align="center">
<a href="https://twitter.com/_superAGI" target="blank">
<img src="https://img.shields.io/twitter/follow/_superAGI?label=Follow: _superAGI&style=social" alt="Follow _superAGI"/>
</a>
<a href="https://www.reddit.com/r/Super_AGI" target="_blank"><img src="https://img.shields.io/twitter/url?label=/r/Super_AGI&logo=reddit&style=social&url=https://github.com/TransformerOptimus/SuperCoder"/></a>

<a href="https://discord.gg/pmFVyCDDyH" target="blank">
<img src="https://img.shields.io/discord/1107593006032355359?label=Join%20SuperAGI&logo=discord&style=social" alt="Join SuperAGI Discord Community"/>
</a>
<a href="https://www.youtube.com/@_superagi" target="_blank"><img src="https://img.shields.io/twitter/url?label=Youtube&logo=youtube&style=social&url=https://github.com/TransformerOptimus/SuperAGI"/></a>
</p>

<p align="center"><b>Connect with the Creator </b></p>

<p align="center">
<a href="https://twitter.com/ishaanbhola" target="blank">
<img src="https://img.shields.io/twitter/follow/ishaanbhola?label=Follow: ishaanbhola&style=social" alt="Follow ishaanbhola"/>
</a>
</p>

<p align="center"><b>Share SuperCoder Repository</b></p>

<p align="center">

<a href="https://x.com/intent/post?text=Check+this+GitHub+repository+out.+SuperCoder+-+The+Future+of+Autonomous+Software+Development.+&url=https%3A%2F%2Fgithub.com%2FTransformerOptimus%2FSuperCoder&hashtags=SuperCoder%2CSuperAGI%2CAGI%2Cfuture" target="blank">
<img src="https://img.shields.io/twitter/follow/_superAGI?label=Share Repo on Twitter&style=social" alt="Follow _superAGI"/></a> 
<a href="https://t.me/share/url?text=Check%20this%20GitHub%20repository%20out.%20SuperCoder%20-%20The%20Future%20of%20Autonomous%20Software%20Development.&url=https://github.com/TransformerOptimus/SuperCoder" target="_blank"><img src="https://img.shields.io/twitter/url?label=Telegram&logo=Telegram&style=social&url=https://github.com/TransformerOptimus/SuperCoder" alt="Share on Telegram"/></a>
<a href="https://api.whatsapp.com/send?text=Check%20this%20GitHub%20repository%20out.%20SuperCoder%20-%20The%20Future%20of%20Autonomous%20Software%20Development.%20https://github.com/TransformerOptimus/SuperCoder"><img src="https://img.shields.io/twitter/url?label=whatsapp&logo=whatsapp&style=social&url=https://github.com/TransformerOptimus/SuperCoder" /></a> <a href="https://www.reddit.com/submit?url=https://github.com/TransformerOptimus/SuperCoder&title=Check%20this%20GitHub%20repository%20out.%20SuperCoder%20-%20The%20Future%20of%20Autonomous%20Software%20Development.
" target="blank">
<img src="https://img.shields.io/twitter/url?label=Reddit&logo=Reddit&style=social&url=https://github.com/TransformerOptimus/SuperCoder" alt="Share on Reddit"/>
</a> <a href="mailto:?subject=Check%20this%20GitHub%20repository%20out.&body=SuperCoder%20-%20The%20Future%20of%20Autonomous%20Software%20Development.%3A%0Ahttps://github.com/TransformerOptimus/SuperCoder" target="_blank"><img src="https://img.shields.io/twitter/url?label=Gmail&logo=Gmail&style=social&url=https://github.com/TransformerOptimus/SuperCoder"/></a>

</p>

<hr>

# SuperCoder: Open Source Autonomous Software Development System

## What are we?

SuperCoder is an autonomous software development system that leverages advanced AI tools and agents to streamline and automate coding, testing, and deployment tasks, enhancing efficiency and reliability.

## 🛠 Supported Languages & Frameworks

SuperCoder 2.0 supports a variety of languages and frameworks for diverse development needs.

<a href="https://www.superagi.com/" target="_blank"><img src=https://superagi.com/wp-content/uploads/2024/07/flask.png height=50px width=50px alt="Flask" valign="middle" title="Flask"></a>
<a href="https://www.superagi.com/" target="_blank"><img src=https://superagi.com/wp-content/uploads/2024/07/django.png height=50px width=50px alt="Django" valign="middle" title="Django"></a>
<a href="https://www.superagi.com/" target="_blank"><img src=https://superagi.com/wp-content/uploads/2024/07/nextjs.png height=50px width=50px alt="NextJS" valign="middle" title="NextJS"></a> 


## ⚙️ Installation

### Prerequisites
Before you proceed, ensure that you have the following installed on your system:
- Docker and Docker Compose
- `direnv`



#### Installing Direnv
To handle environment variables more efficiently, install `direnv`:
```bash
# For macOS
brew install direnv

# For Ubuntu
sudo apt-get install direnv
```

After installation, hook direnv into your shell:

```bash 
echo 'eval "$(direnv hook bash)"' >> ~/.bashrc
source ~/.bashrc
```

If you are using a shell other than bash, replace bash with your specific shell (e.g., zsh or fish).

*Note: direnv is one of the suggested ways other ways to setup environment variables are also possible*

## 🖥️ Setup

To get started with SuperCoder, first follow these steps:

1. Open your terminal and clone the SuperCoder repository.
```
git clone https://github.com/TransformerOptimus/SuperCoder.git 
```

2. Navigate to the cloned repository directory using the command:
```
cd SuperCoder
```

3. Environment Configuration

Create a .envrc file in the root directory of your project and populate it with the necessary environment variables:
```bash
export AI_DEVELOPER_GITNESS_TOKEN=
export AI_DEVELOPER_GITNESS_URL=https://gitness:3000
export AI_DEVELOPER_GITNESS_HOST=gitness:3000
export AI_DEVELOPER_GITNESS_USER=
export AI_DEVELOPER_OPENAI_API_KEY=
export AI_DEVELOPER_WORKSPACE_WORKING_DIR=/workspaces
export AI_DEVELOPER_WORKSPACE_TEMPLATE_DIR=/templates/python
export AI_DEVELOPER_WORKSPACE_SERVICE_ENDPOINT=http://ws:8080
export AI_DEVELOPER_APP_URL=
export AI_DEVELOPER_GITNESS_PASSWORD=admin
export AI_DEVELOPER_GITNESS_USER=admin
export AI_DEVELOPER_GITHUB_CLIENT_SECRET=
export AI_DEVELOPER_GITHUB_CLIENT_ID=

export NEW_RELIC_ENABLED=false

export WORKSPACES_GITNESS_USER=
export WORKSPACES_GITNESS_TOKEN=
```

To allow direnv to load these settings, run:

```bash
direnv allow .
```

4. Ensure that Docker is installed on your system. You can download and install it from [here](https://docs.docker.com/get-docker/).


5. Once you have Docker Desktop running, run the following command in the AutoNode directory:

```bash
docker-compose up --build
```

6. Open your web browser and navigate to http://localhost:3000 to access the UI and make sure your server is running.

### 📖 Need Help?

Join our [Discord community](https://discord.gg/pmFVyCDDyH) for support and discussions.

[![Join us on Discord](https://invidget.switchblade.xyz/pmFVyCDDyH)](https://discord.gg/pmFVyCDDyH)

If you have questions or encounter issues, please don't hesitate to [create a new issue](https://github.com/TransformerOptimus/SuperCoder/issues/new/choose) to get support or reach out to support@superagi.com.


### ⚠️ Under Development!
This project is under active development and may still have issues. We appreciate your understanding and patience. If you encounter any problems, please check the open issues first. If your issue is not listed, kindly create a new issue detailing the error or problem you experienced. Thank you for your support!
