// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generators

import (
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
)

// copyStatic is a generic Generator that copies a list of static assets over
// to the new assets directory.
type copyStatic struct {
	assets  []string
	sources []string
}

// Generate moves a static list of static assets from the assets
// directory to the output directory. If the assets directory
// and the output directory are identical, it does nothing.
func (c *copyStatic) Generate(cfg *common.GenConfig) error {
	if cfg.AssetsDir != cfg.OutputDir {
		if err := copyFiles(cfg.OutputDir, cfg.AssetsDir, c.assets); err != nil {
			return err
		}
	}
	return copyFiles(cfg.OutputDir, cfg.SourceAssetsDir, c.sources)
}

func copyFiles(dstPath, srcPath string, relPaths []string) error {
	for _, relPath := range relPaths {
		outputPath := filepath.Join(dstPath, relPath)
		inputPath := filepath.Join(srcPath, relPath)
		err := fileutil.CopyFile(outputPath, inputPath, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func BiogoIgor() common.Generator {
	return &copyStatic{assets: []string{
		"Homo_sapiens.GRCh38.dna.chromosome.22.gff",
		"README.md",
		"LICENSE",
	}}
}

func BiogoKrishna() common.Generator {
	return &copyStatic{assets: []string{
		"Mus_musculus.GRCm38.dna.nonchromosomal.fa",
		"README.md",
		"LICENSE",
	}}
}

const wikiDumpName = "enwiki-20080103-pages-articles.xml.bz2"

func BleveIndex() common.Generator {
	return &copyStatic{assets: []string{
		wikiDumpName,
		"README.md",
		"LICENSE",
	}}
}

func FoglemanFauxGL() common.Generator {
	return &copyStatic{assets: []string{
		"3dbenchy.stl",
		"README.md",
		"LICENSE",
	}}
}

func FoglemanPT() common.Generator {
	return &copyStatic{assets: []string{
		"gopher.mtl",
		"gopher.obj",
		"README.md",
		"LICENSE",
	}}
}

func GopherLua() common.Generator {
	return &copyStatic{
		assets: []string{
			"input.txt",
			"README.md",
			"LICENSE",
			"k-nucleotide.lua",
		},
	}
}

func Markdown() common.Generator {
	return &copyStatic{assets: []string{
		"AchoArnold_discount-for-student-dev_README.md",
		"agarrharr_awesome-cli-apps_README.md",
		"agarrharr_awesome-macos-screensavers_README.md",
		"agarrharr_awesome-static-website-services_README.md",
		"alferov_awesome-gulp_README.md",
		"analyticalmonk_awesome-neuroscience_README.md",
		"angrykoala_awesome-esolangs_README.md",
		"avajs_awesome-ava_README.md",
		"aviaryan_awesome-no-login-web-apps_README.md",
		"beaconinside_awesome-beacon_README.md",
		"benoitjadinon_awesome-xamarin_README.md",
		"bfred-it_Awesome-WebExtensions_README.md",
		"brabadu_awesome-fonts_README.md",
		"briatte_awesome-network-analysis_README.md",
		"browserify_awesome-browserify_README.md",
		"brunocvcunha_awesome-userscripts_README.md",
		"BubuAnabelas_awesome-markdown_README.md",
		"burningtree_awesome-json_README.md",
		"candelibas_awesome-ionic_README.md",
		"chentsulin_awesome-graphql_README.md",
		"choojs_awesome-choo_README.md",
		"christian-bromann_awesome-selenium_README.md",
		"ChristosChristofidis_awesome-deep-learning_README.md",
		"ciconia_awesome-music_README.md",
		"Codepoints_awesome-codepoints_README.md",
		"cristianoliveira_awesome4girls_README.md",
		"CUTR-at-USF_awesome-transit_README.md",
		"cyberglot_awesome-answers_README.md",
		"cyclejs-community_awesome-cyclejs_README.md",
		"d3viant0ne_awesome-rethinkdb_README.md",
		"danielecook_Awesome-Bioinformatics_README.md",
		"dav009_awesome-spanish-nlp_README.md",
		"DavidLambauer_awesome-magento2_README.md",
		"deanhume_typography_README.md",
		"diessica_awesome-sketch_README.md",
		"dok_awesome-text-editing_README.md",
		"domenicosolazzo_awesome-okr_README.md",
		"drewrwilson_toolsforactivism_README.md",
		"dustinspecker_awesome-eslint_README.md",
		"dylanrees_citizen-science_README.md",
		"eleventigers_awesome-rxjava_README.md",
		"enaqx_awesome-react_README.md",
		"exAspArk_awesome-chatops_README.md",
		"Famolus_awesome-sass_README.md",
		"fasouto_awesome-dataviz_README.md",
		"fcambus_nginx-resources_README.md",
		"felipebueno_awesome-PICO-8_README.md",
		"feross_awesome-mad-science_README.md",
		"filipelinhares_awesome-slack_README.md",
		"fliptheweb_motion-ui-design_README.md",
		"Fr0sT-Brutal_awesome-delphi_README.md",
		"gamontal_awesome-katas_README.md",
		"gdi2290_awesome-angular_README.md",
		"Granze_awesome-polymer_README.md",
		"guillaume-chevalier_awesome-deep-learning-resources_README.md",
		"hackerkid_bots_README.md",
		"hackerkid_Mind-Expanding-Books_README.md",
		"hantuzun_awesome-clojurescript_README.md",
		"harpribot_awesome-information-retrieval_README.md",
		"hbokh_awesome-saltstack_README.md",
		"heynickc_awesome-ddd_README.md",
		"hobbyquaker_awesome-mqtt_README.md",
		"HQarroum_awesome-iot_README.md",
		"igorbarinov_awesome-bitcoin_README.md",
		"igorbarinov_awesome-data-engineering_README.md",
		"iJackUA_awesome-vagrant_README.md",
		"inspectit-labs_awesome-inspectit_README.md",
		"ipfs_awesome-ipfs_README.md",
		"isRuslan_awesome-elm_README.md",
		"jagracey_Awesome-Unicode_README.md",
		"jakoch_awesome-composer_README.md",
		"JanVanRyswyck_awesome-talks_README.md",
		"jbhuang0604_awesome-computer-vision_README.md",
		"jbmoelker_progressive-enhancement-resources_README.md",
		"jdorfman_awesome-json-datasets_README.md",
		"jdrgomes_awesome-postcss_README.md",
		"JesseTG_awesome-qt_README.md",
		"joaomilho_awesome-idris_README.md",
		"jonathandion_awesome-emails_README.md",
		"jwaterfaucett_awesome-foss-apps_README.md",
		"karlhorky_learn-to-program_README.md",
		"kdeldycke_awesome-falsehood_README.md",
		"KotlinBy_awesome-kotlin_README.md",
		"krispo_awesome-haskell_README.md",
		"LappleApple_awesome-leading-and-managing_README.md",
		"LewisJEllis_awesome-lua_README.md",
		"LucasBassetti_awesome-less_README.md",
		"lucasviola_awesome-functional-programming_README.md",
		"lucasviola_awesome-tech-videos_README.md",
		"lukasz-madon_awesome-remote-job_README.md",
		"machinomy_awesome-non-financial-blockchain_README.md",
		"mailtoharshit_awesome-salesforce_README.md",
		"mark-rushakoff_awesome-influxdb_README.md",
		"matiassingers_awesome-readme_README.md",
		"matiassingers_awesome-slack_README.md",
		"MaximAbramchuck_awesome-interview-questions_README.md",
		"melvin0008_awesome-projects-boilerplates_README.md",
		"mfornos_awesome-microservices_README.md",
		"micromata_awesome-javascript-learning_README.md",
		"mmccaff_PlacesToPostYourStartup_README.md",
		"mohataher_awesome-tinkerpop_README.md",
		"motion-open-source_awesome-rubymotion_README.md",
		"moul_awesome-ssh_README.md",
		"mre_awesome-static-analysis_README.md",
		"MunGell_awesome-for-beginners_README.md",
		"neueda_awesome-neo4j_README.md",
		"neutraltone_awesome-stock-resources_README.md",
		"nicolesaidy_awesome-web-design_README.md",
		"nikgraf_awesome-draft-js_README.md",
		"nirgn975_awesome-drupal_README.md",
		"nmec_awesome-ember_README.md",
		"NoahBuscher_Inspire_README.md",
		"notthetup_awesome-webaudio_README.md",
		"ooade_awesome-preact_README.md",
		"owainlewis_awesome-artificial-intelligence_README.md",
		"parro-it_awesome-micro-npm-packages_README.md",
		"passy_awesome-purescript_README.md",
		"pazguille_offline-first_README.md",
		"pehapkari_awesome-symfony-education_README.md",
		"PerfectCarl_awesome-play1_README.md",
		"petk_awesome-dojo_README.md",
		"petk_awesome-jquery_README.md",
		"PhantomYdn_awesome-wicket_README.md",
		"podo_awesome-framer_README.md",
		"qazbnm456_awesome-web-security_README.md",
		"quozd_awesome-dotnet_README.md",
		"ramnes_awesome-mongodb_README.md",
		"refinerycms-contrib_awesome-refinerycms_README.md",
		"RichardLitt_awesome-conferences_README.md",
		"RichardLitt_awesome-fantasy_README.md",
		"RichardLitt_awesome-styleguides_README.md",
		"roaldnefs_awesome-prometheus_README.md",
		"rossant_awesome-math_README.md",
		"rust-unofficial_awesome-rust_README.md",
		"RyanZim_awesome-npm-scripts_README.md",
		"scholtzm_awesome-steam_README.md",
		"seancoyne_awesome-coldfusion_README.md",
		"sfischer13_awesome-eta_README.md",
		"sfischer13_awesome-frege_README.md",
		"sfischer13_awesome-ledger_README.md",
		"shuaibiyy_awesome-terraform_README.md",
		"siboehm_awesome-learn-datascience_README.md",
		"Siddharth11_Colorful_README.md",
		"sindresorhus_amas_README.md",
		"sindresorhus_awesome-electron_README.md",
		"sindresorhus_awesome-nodejs_README.md",
		"sindresorhus_awesome-npm_README.md",
		"sindresorhus_awesome-observables_README.md",
		"sindresorhus_awesome_README.md",
		"sindresorhus_awesome-scifi_README.md",
		"sindresorhus_awesome-tap_README.md",
		"sindresorhus_quick-look-plugins_README.md",
		"sitepoint-editors_awesome-symfony_README.md",
		"sjfricke_awesome-webgl_README.md",
		"sorrycc_awesome-javascript_README.md",
		"springload_awesome-wagtail_README.md",
		"SrinivasanTarget_awesome-appium_README.md",
		"standard_awesome-standard_README.md",
		"stetso_awesome-gideros_README.md",
		"stevemao_awesome-git-addons_README.md",
		"stoeffel_awesome-ama-answers_README.md",
		"stoeffel_awesome-fp-js_README.md",
		"stve_awesome-dropwizard_README.md",
		"sublimino_awesome-funny-markov_README.md",
		"terkelg_awesome-creative-coding_README.md",
		"terryum_awesome-deep-learning-papers_README.md",
		"thangchung_awesome-dotnet-core_README.md",
		"TheJambo_awesome-testing_README.md",
		"thibmaek_awesome-raspberry-pi_README.md",
		"tmcw_awesome-geojson_README.md",
		"tobiasbueschel_awesome-pokemon_README.md",
		"unicodeveloper_awesome-lumen_README.md",
		"unicodeveloper_awesome-nextjs_README.md",
		"uralbash_awesome-pyramid_README.md",
		"vhpoet_awesome-ripple_README.md",
		"viatsko_awesome-vscode_README.md",
		"vinkla_awesome-fuse_README.md",
		"vinkla_shareable-links_README.md",
		"vitalets_awesome-smart-tv_README.md",
		"vorpaljs_awesome-vorpal_README.md",
		"vredniy_awesome-newsletters_README.md",
		"vuejs_awesome-vue_README.md",
		"watson_awesome-computer-history_README.md",
		"webpro_awesome-dotfiles_README.md",
		"yenchenlin_awesome-watchos_README.md",
		"yissachar_awesome-dart_README.md",
		"yrgo_awesome-eg_README.md",
		"README.md",
		"LICENSE",
	}}
}
